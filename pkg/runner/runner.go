package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// EventTime - time to unmarshal nano time.
type EventTime struct {
	time.Time
}

// UnmarshalJSON - override unmarshal json.
func (e *EventTime) UnmarshalJSON(b []byte) (err error) {
	e.Time, err = time.Parse("2006-01-02T15:04:05.999999999", strings.Trim(string(b[:]), "\"\\"))
	if err != nil {
		return err
	}
	return nil
}

// MarshalJSON - override the marshal json.
func (e EventTime) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", e.Time.Format("2006-01-02T15:04:05.99999999"))), nil
}

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
	Created   EventTime              `json:"created"`
}

// StatusJobEvent - event of an ansible run.
type StatusJobEvent struct {
	UUID      string         `json:"uuid"`
	Counter   int            `json:"counter"`
	StdOut    string         `json:"stdout"`
	StartLine int            `json:"start_line"`
	EndLine   int            `json:"EndLine"`
	Event     string         `json:"event"`
	EventData StatsEventData `json:"event_data"`
	PID       int            `json:"pid"`
	Created   EventTime      `json:"created"`
}

// StatsEventData - data for a the status event.
type StatsEventData struct {
	Playbook     string         `json:"playbook"`
	PlaybookUUID string         `json:"playbook_uuid"`
	Changed      map[string]int `json:"changed"`
	Ok           map[string]int `json:"ok"`
	Failures     map[string]int `json:"failures"`
	Skipped      map[string]int `json:"skipped"`
}

// Runner - a runnable that should take the parameters and name and namespace
// and run the correct code.
type Runner interface {
	Run(map[string]interface{}, string, string) (*StatusJobEvent, error)
}

// Playbook - playbook type of runner.
type Playbook struct {
	Path string
	GVK  schema.GroupVersionKind
}

// Run - This should allow the playbook runner to run.
func (p *Playbook) Run(parameters map[string]interface{}, name, namespace string) (*StatusJobEvent, error) {
	parameters["meta"] = map[string]string{"namespace": namespace, "name": name}
	runnerSandbox := fmt.Sprintf("/home/ansible-operator/runner/%s/%s/%s/%s/%s", p.GVK.Group, p.GVK.Version, p.GVK.Kind, namespace, name)
	b, err := json.Marshal(parameters)
	if err != nil {
		return nil, err
	}
	//Write parameters to correct file on disk
	err = createRunnerEnvironment(b, runnerSandbox)
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

	dc := exec.Command("ansible-runner", "-vv", "-p", "playbook.yaml", "-i", fmt.Sprintf("%v", ident), "run", runnerSandbox)
	dc.Stdout = os.Stdout
	dc.Stderr = os.Stderr
	err = dc.Run()
	if err != nil {
		return nil, err
	}
	logrus.Infof("ran: %v for playbook: %v", ident, p.Path)
	logrus.Infof("collecting results for run %v", ident)

	eventFiles, err := ioutil.ReadDir(fmt.Sprintf("%v/artifacts/%v/job_events", runnerSandbox, ident))
	if err != nil {
		return nil, err
	}
	if len(eventFiles) == 0 {
		return nil, fmt.Errorf("Unable to read event data")
	}
	sort.Sort(fileInfos(eventFiles))
	//get the last event, which should be a status.
	d, err := ioutil.ReadFile(fmt.Sprintf("%v/artifacts/%v/job_events/%v", runnerSandbox, ident, eventFiles[len(eventFiles)-1].Name()))
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

// Role - role type of runner
type Role struct {
	name string
}

func createRunnerEnvironment(parameters []byte, runnerSandbox string) error {
	err := os.MkdirAll(fmt.Sprintf("%v/env", runnerSandbox), os.ModePerm)
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
