package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Runner - a runnable that should take the parameters and name and namespace
// and run the correct code.
type Runner interface {
	Run(map[string]interface{}, string, string, string) (*StatusJobEvent, error)
}

// Playbook - playbook type of runner.
type Playbook struct {
	Path string
	GVK  schema.GroupVersionKind
}

// Run - This should allow the playbook runner to run.
func (p *Playbook) Run(parameters map[string]interface{}, name, namespace, kubeconfig string) (*StatusJobEvent, error) {
	parameters["meta"] = map[string]string{"namespace": namespace, "name": name}
	runnerSandbox := fmt.Sprintf("/home/ansible-operator/runner/%s/%s/%s/%s/%s", p.GVK.Group, p.GVK.Version, p.GVK.Kind, namespace, name)
	b, err := json.Marshal(parameters)
	if err != nil {
		return nil, err
	}
	//Write parameters to correct file on disk
	err = createRunnerEnvironment(b, runnerSandbox, kubeconfig)
	if err != nil {
		return nil, err
	}
	//copy playbook from the path to the project of the runnerEnvironment
	err = copyFile(p.Path, fmt.Sprintf("%v/project/playbook.yaml", runnerSandbox))
	if err != nil {
		return nil, err
	}
	ident := rand.Int()
	logrus.Infof("running: %v for playbook: %v", ident, p.Path)

	dc := exec.Command("ansible-runner", "-vvvvv", "-p", "playbook.yaml", "-i", fmt.Sprintf("%v", ident), "run", runnerSandbox)
	dc.Env = append(os.Environ(), fmt.Sprintf("K8S_AUTH_KUBECONFIG=%s", kubeconfig))
	dc.Env = append(os.Environ(), fmt.Sprintf("CALLBACK_SOCKET=%v/%v.sock", runnerSandbox, ident))
	dc.Stdout = os.Stdout
	dc.Stderr = os.Stderr
	errChannel := make(chan error)
	cancel := make(chan struct{})
	w, err := NewWatcher(runnerSandbox, fmt.Sprintf("%v", ident))
	if err != nil {
		return nil, err
	}
	go func() {
		err := dc.Run()
		errChannel <- err
	}()
	for {
		select {
		case je := <-w.JobEvents:
			logrus.Infof("event UUID: %v event: %v stdout: %v", je.UUID, je.Event, je.StdOut)
			if je.Event == "playbook_on_stats" {
				logrus.Infof("ran: %v for playbook: %v", ident, p.Path)
				logrus.Infof("collecting results for run %v", ident)
				d, err := json.Marshal(je)
				if err != nil {
					return nil, err
				}
				o := &StatusJobEvent{}
				err = json.Unmarshal(d, o)
				if err != nil {
					return nil, err
				}
				cancel <- struct{}{}
				return o, nil
			}
		case err := <-errChannel:
			if err != nil {
				cancel <- struct{}{}
				return nil, err
			}
		case <-time.After(10 * time.Minute):
			cancel <- struct{}{}
			return nil, fmt.Errorf("timeout of 10 minutes was reached")
		}
	}
}

// Role - role type of runner
type Role struct {
	name string
}

// Helper functions for the runner.

func createRunnerEnvironment(parameters []byte, runnerSandbox, configPath string) error {
	err := os.MkdirAll(fmt.Sprintf("%v/env", runnerSandbox), os.ModePerm)
	if err != nil {
		logrus.Errorf("unable to create runner directory - %v", runnerSandbox)
		return err
	}
	err = ioutil.WriteFile(fmt.Sprintf("%v/env/envvars", runnerSandbox), []byte(
		fmt.Sprintf("---\nK8S_AUTH_KUBECONFIG=%s", configPath)), 0644,
	)
	if err != nil {
		logrus.Errorf("unable to create extravars file - %v", runnerSandbox)
		return err
	}
	err = os.MkdirAll(fmt.Sprintf("%v/project", runnerSandbox), os.ModePerm)
	if err != nil {
		logrus.Errorf("unable to create project directory - %v", runnerSandbox)
		return err
	}
	err = os.MkdirAll(fmt.Sprintf("%v/inventory", runnerSandbox), os.ModePerm)
	if err != nil {
		logrus.Errorf("unable to create inventory directory - %v", runnerSandbox)
		return err
	}
	err = ioutil.WriteFile(fmt.Sprintf("%v/inventory/hosts", runnerSandbox), []byte("localhost ansible_connection=local"), 0644)
	if err != nil {
		logrus.Errorf("unable to create extravars file - %v", runnerSandbox)
		return err
	}
	err = ioutil.WriteFile(fmt.Sprintf("%v/env/extravars", runnerSandbox), parameters, 0644)
	if err != nil {
		logrus.Errorf("unable to create extravars file - %v", runnerSandbox)
		return err
	}
	return nil
}

// Copy the src file to dst. Any existing file will be overwritten and will not
// copy file attributes.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

type fileInfos []os.FileInfo

func (f fileInfos) Len() int {
	return len(f)
}

func (f fileInfos) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}

func (f fileInfos) Less(i, j int) bool {
	//Strip into part of filename
	iInt, err := strconv.Atoi(strings.Split(f[i].Name(), "-")[0])
	if err != nil {
		return false
	}
	jInt, err := strconv.Atoi(strings.Split(f[j].Name(), "-")[0])
	if err != nil {
		return false
	}
	return iInt < jInt
}
