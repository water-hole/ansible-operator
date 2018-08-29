package runner

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/water-hole/ansible-operator/pkg/paramconv"
	"github.com/water-hole/ansible-operator/pkg/runner/eventapi"
	"github.com/water-hole/ansible-operator/pkg/runner/internal/inputdir"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Runner - a runnable that should take the parameters and name and namespace
// and run the correct code.
type Runner interface {
	Run(*unstructured.Unstructured, string) (chan eventapi.JobEvent, error)
	GetFinalizer() (string, bool)
}

// watch holds data used to create a mapping of GVK to ansible playbook or role.
// The mapping is used to compose an ansible operator.
type watch struct {
	Version   string     `yaml:"version"`
	Group     string     `yaml:"group"`
	Kind      string     `yaml:"kind"`
	Playbook  string     `yaml:"playbook"`
	Role      string     `yaml:"role"`
	Finalizer *finalizer `yaml:"finalizer"`
}

type finalizer struct {
	Name     string                 `yaml:"name"`
	Playbook string                 `yaml:"playbook"`
	Role     string                 `yaml:"role"`
	Vars     map[string]interface{} `yaml:"vars"`
	Runner   Runner
}

// NewFromWatches reads the operator's config file at the provided path.
func NewFromWatches(path string) (map[schema.GroupVersionKind]Runner, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		logrus.Errorf("failed to get config file %v", err)
		return nil, err
	}
	watches := []watch{}
	err = yaml.Unmarshal(b, &watches)
	if err != nil {
		logrus.Errorf("failed to unmarshal config %v", err)
		return nil, err
	}

	m := map[schema.GroupVersionKind]Runner{}
	for _, w := range watches {
		s := schema.GroupVersionKind{
			Group:   w.Group,
			Version: w.Version,
			Kind:    w.Kind,
		}
		// Check if schema is a duplicate
		if _, ok := m[s]; ok {
			return nil, fmt.Errorf("duplicate GVK: %v", s.String())
		}
		switch {
		case w.Playbook != "":
			if !filepath.IsAbs(w.Playbook) {
				return nil, fmt.Errorf("Path must be absolute for %v", s)
			}
			run = NewForPlaybook(w.Playbook, s).(*runner)
		case w.Role != "":
			if !filepath.IsAbs(w.Role) {
				return nil, fmt.Errorf("Path must be absolute for %v", s)
			}
			run = NewForRole(w.Role, s).(*runner)
		default:
			return nil, fmt.Errorf("Either playbook or role must be defined for %v", s)
		}

		switch {
		case w.Finalizer == nil:
		case w.Finalizer.Playbook != "":
			if !filepath.IsAbs(w.Finalizer.Playbook) {
				return nil, fmt.Errorf("Path must be absolute for finalizer on %v", s)
			}
			w.Finalizer.Runner = NewForPlaybook(w.Finalizer.Playbook, s)
		case w.Finalizer.Role != "":
			if !filepath.IsAbs(w.Finalizer.Role) {
				return nil, fmt.Errorf("Path must be absolute for finalizer on %v", s)
			}
			w.Finalizer.Runner = NewForRole(w.Finalizer.Role, s)
		case len(w.Finalizer.Vars) != 0:
			w.Finalizer.Runner = run
		}
		run.Finalizer = w.Finalizer
		m[s] = run
	}
	return m, nil
}

// NewForPlaybook returns a new Runner based on the path to an ansible playbook.
func NewForPlaybook(path string, gvk schema.GroupVersionKind) Runner {
	return &runner{
		Path: path,
		GVK:  gvk,
		cmdFunc: func(ident, inputDirPath string) *exec.Cmd {
			dc := exec.Command("ansible-runner", "-vv", "-p", path, "-i", ident, "run", inputDirPath)
			return dc
		},
	}
}

// NewForRole returns a new Runner based on the path to an ansible role.
func NewForRole(path string, gvk schema.GroupVersionKind) Runner {
	path = strings.TrimRight(path, "/")
	return &runner{
		Path: path,
		GVK:  gvk,
		cmdFunc: func(ident, inputDirPath string) *exec.Cmd {
			rolePath, roleName := filepath.Split(path)
			dc := exec.Command("ansible-runner", "-vv", "--role", roleName, "--roles-path", rolePath, "--hosts", "localhost", "-i", ident, "run", inputDirPath)
			return dc
		},
	}
}

// runner - implements the Runner interface for a GVK that's being watched.
type runner struct {
	Path      string                                     // path on disk to a playbook or role depending on what cmdFunc expects
	GVK       schema.GroupVersionKind                    // GVK being watched that corresponds to the Path
	cmdFunc   func(ident, inputDirPath string) *exec.Cmd // returns a Cmd that runs ansible-runner
	Finalizer *finalizer
}

func (r *runner) Run(u *unstructured.Unstructured, kubeconfig string) (chan eventapi.JobEvent, error) {
	ident := strconv.Itoa(rand.Int())
	logger := logrus.WithFields(logrus.Fields{
		"component": "runner",
		"job":       ident,
		"name":      u.GetName(),
		"namespace": u.GetNamespace(),
	})
	// start the event receiver. We'll check errChan for an error after
	// ansible-runner exits.
	errChan := make(chan error, 1)
	receiver, err := eventapi.New(ident, errChan)
	if err != nil {
		return nil, err
	}
	inputDir := inputdir.InputDir{
		Path:       filepath.Join("/tmp/ansible-operator/runner/", r.GVK.Group, r.GVK.Version, r.GVK.Kind, u.GetNamespace(), u.GetName()),
		Parameters: r.makeParameters(u),
		EnvVars: map[string]string{
			"K8S_AUTH_KUBECONFIG": kubeconfig,
		},
		Settings: map[string]string{
			"runner_http_url":  receiver.SocketPath,
			"runner_http_path": receiver.URLPath,
		},
	}
	// If Path is a dir, assume it is a role path. Otherwise assume it's a
	// playbook path
	fi, err := os.Lstat(r.Path)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		inputDir.PlaybookPath = r.Path
	}
	err = inputDir.Write()
	if err != nil {
		return nil, err
	}

	go func() {
		var dc *exec.Cmd
		deleted := u.GetDeletionTimestamp() != nil
		if deleted {
			if r.isFinalizerRun(u) {
				logger.Debugf("Resource is marked for deletion, running finalizer %s", r.Finalizer.Name)
				dc = r.Finalizer.Runner.(*runner).cmdFunc(ident, inputDir.Path)
			} else {
				logger.Debug("Resource has been deleted, but no finalizer was matched, skipping reconciliation")
				receiver.Close()
				return
			}
		} else {
			dc = r.cmdFunc(ident, inputDir.Path)
		}

		err := dc.Run()
		if err != nil {
			logger.Errorf("error from ansible-runner: %s", err.Error())
		} else {
			logger.Info("ansible-runner exited successfully")
		}

		receiver.Close()
		err = <-errChan
		// http.Server returns this in the case of being closed cleanly
		if err != nil && err != http.ErrServerClosed {
			logger.Errorf("error from event api: %s", err.Error())
		}
	}()
	return receiver.Events, nil
}

func (r *runner) GetFinalizer() (string, bool) {
	if r.Finalizer != nil {
		return r.Finalizer.Name, true
	}
	return "", false
}

func (r *runner) isFinalizerRun(u *unstructured.Unstructured) bool {
	finalizersSet := r.Finalizer != nil && u.GetFinalizers() != nil
	// If finalizers are set and we find our finalizer in them, we need to run the finalizer
	if finalizersSet && u.GetDeletionTimestamp() != nil {
		for _, f := range u.GetFinalizers() {
			if f == r.Finalizer.Name {
				return true
			}
		}
	}
	return false
}

func (r *runner) makeParameters(u *unstructured.Unstructured) map[string]interface{} {
	s := u.Object["spec"]
	spec, ok := s.(map[string]interface{})
	if !ok {
		logrus.Warnf("spec was not found for CR:%v - %v in %v", u.GroupVersionKind(), u.GetNamespace(), u.GetName())
		spec = map[string]interface{}{}
	}
	parameters := paramconv.MapToSnake(spec)
	parameters["meta"] = map[string]string{"namespace": u.GetNamespace(), "name": u.GetName()}
	objectKey := fmt.Sprintf("_%v_%v", strings.Replace(r.GVK.Group, ".", "_", -1), strings.ToLower(r.GVK.Kind))
	parameters[objectKey] = u.Object
	if r.isFinalizerRun(u) {
		for k, v := range r.Finalizer.Vars {
			parameters[k] = v
		}
	}
	return parameters
}
