package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// JobEvent - event of an ansible run.
type JobEvent struct {
	UUID      string                 `json:"uuid"`
	Counter   int                    `json:"counter"`
	StdOut    string                 `json:"stdout"`
	StartLine int                    `json:"start_line"`
	EndLine   int                    `json:"EndLine"`
	Event     string                 `json:"event"`
	EventData map[string]interface{} `json:"event_data"`
	PID       int                    `json:"pid"`
	Created   time.Time
}

func ReadEvents(dir, ident string) ([]JobEvent, error) {
	return nil, nil

}

type Runner interface {
	Run(map[string]interface{}, string, string) error
}

type Playbook struct {
	Path string
	GVK  schema.GroupVersionKind
}

// Run - This should allow the playbook runner to run.
func (p *Playbook) Run(parameters map[string]interface{}, name, namespace string) error {
	runnerSandbox := fmt.Sprintf("/home/ansible-operator/runner/%s/%s/%s/%s/%s", p.GVK.Group, p.GVK.Version, p.GVK.Kind, namespace, name)
	b, err := json.Marshal(parameters)
	if err != nil {
		return err
	}
	//Write parameters to correct file on disk
	err = createRunnerEnvironment(b, runnerSandbox)
	if err != nil {
		return err
	}
	//copy playbook from the path to the project of the runnerEnvironment
	err = copyFile(p.Path, fmt.Sprintf("%v/project/playbook.yaml", runnerSandbox))
	if err != nil {
		return err
	}

	dc := exec.Command("ansible-runner", "-vv", "-p", "playbook.yaml", "run", runnerSandbox)
	dc.Stdout = os.Stdout
	dc.Stderr = os.Stderr
	return dc.Run()
}

type Role struct {
	name string
}

func createRunnerEnvironment(parameters []byte, runnerSandbox string) error {
	err := os.RemoveAll(runnerSandbox)
	if err != nil {
		logrus.Errorf("unable to remove sandbox for %v", runnerSandbox)
		return err
	}
	err = os.MkdirAll(fmt.Sprintf("%v/env", runnerSandbox), os.ModePerm)
	if err != nil {
		logrus.Errorf("unable to create runner directory - %v", runnerSandbox)
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
