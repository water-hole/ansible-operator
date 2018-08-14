package runner

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/water-hole/ansible-operator/pkg/paramconv"
	"github.com/water-hole/ansible-operator/pkg/runner/internal/inputdir"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Runner - a runnable that should take the parameters and name and namespace
// and run the correct code.
type Runner interface {
	Run(map[string]interface{}, string, string, string) (*StatusJobEvent, error)
}

// watch holds data used to create a mapping of GVK to ansible playbook or role.
// The mapping is used to compose an ansible operator.
type watch struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Group   string `yaml:"group"`
	Kind    string `yaml:"kind"`
	Path    string `yaml:"path"`
}

// NewFromConfig reads the operator's config file at the provided path.
func NewFromConfig(path string) (map[schema.GroupVersionKind]Runner, error) {
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
		m[s] = NewForPlaybook(w.Path, s)
	}
	return m, nil
}

// NewForRole returns a new Runner based on the path to an ansible playbook.
func NewForPlaybook(path string, gvk schema.GroupVersionKind) Runner {
	return &runner{
		Path: path,
		GVK:  gvk,
		cmdFunc: func(ident, inputDirPath string) *exec.Cmd {
			dc := exec.Command("ansible-runner", "-vv", "-p", path, "-i", ident, "run", inputDirPath)
			dc.Stdout = os.Stdout
			dc.Stderr = os.Stderr
			return dc
		},
	}
}

// NewForRole returns a new Runner based on the path to an ansible role.
func NewForRole(path string, gvk schema.GroupVersionKind) Runner {
	return &runner{
		Path: path,
		GVK:  gvk,
		cmdFunc: func(ident, inputDirPath string) *exec.Cmd {
			// FIXME the below command does not fully work
			dc := exec.Command("ansible-runner", "-vv", "--role", "busybox", "--roles-path", "/opt/ansible/roles/", "--hosts", "localhost", "-i", ident, "run", inputDirPath)
			dc.Stdout = os.Stdout
			dc.Stderr = os.Stderr
			return dc
		},
	}
}

// runner - implements the Runner interface for a GVK that's being watched.
type runner struct {
	Path    string                                     // path on disk to a playbook or role depending on what cmdFunc expects
	GVK     schema.GroupVersionKind                    // GVK being watched that corresponds to the Path
	cmdFunc func(ident, inputDirPath string) *exec.Cmd // returns a Cmd that runs ansible-runner
}

// Run uses the runner with the given input and returns a status.
func (r *runner) Run(parameters map[string]interface{}, name, namespace, kubeconfig string) (*StatusJobEvent, error) {
	parameters["meta"] = map[string]string{"namespace": namespace, "name": name}
	inputDir := inputdir.InputDir{
		Path:         filepath.Join("/tmp/ansible-operator/runner/", r.GVK.Group, r.GVK.Version, r.GVK.Kind, namespace, name),
		PlaybookPath: r.Path,
		Parameters:   paramconv.MapToSnake(parameters),
		EnvVars: map[string]string{
			"K8S_AUTH_KUBECONFIG": kubeconfig,
		},
	}
	err := inputDir.Write()
	if err != nil {
		return nil, err
	}

	ident := strconv.Itoa(rand.Int())
	dc := r.cmdFunc(ident, inputDir.Path)
	err = dc.Run()
	if err != nil {
		return nil, err
	}

	return jobStatus(inputDir.Path, ident)
}

// Status returns the final status from ansible-runner. The implementation will change to use an
// event API instead of the filesystem inspection seen below.
func jobStatus(path, ident string) (*StatusJobEvent, error) {
	logrus.Infof("collecting results for run %v", ident)

	eventFiles, err := ioutil.ReadDir(filepath.Join(path, "artifacts", ident, "job_events"))
	if err != nil {
		return nil, err
	}
	if len(eventFiles) == 0 {
		return nil, errors.New("Unable to read event data")
	}
	sort.Sort(fileInfos(eventFiles))
	//get the last event, which should be a status.
	d, err := ioutil.ReadFile(filepath.Join(path, "artifacts/", ident, "job_events", eventFiles[len(eventFiles)-1].Name()))
	if err != nil {
		return nil, err
	}
	o := &StatusJobEvent{}
	err = json.Unmarshal(d, o)
	if err != nil {
		return nil, err
	}
	return o, nil
}
