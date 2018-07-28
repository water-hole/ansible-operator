package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// SocketEndString - string that will tell the socket end.
	SocketEndString = "endfile"
)

// Watcher - used to watch the file system
type Watcher struct {
	JobEvents     chan *JobEvent
	runnerSandbox string
	socket        string
}

// NewWatcher - Creates a watcher.
func NewWatcher(runnerSandbox, socket string) (*Watcher, error) {
	logrus.Debugf("starting socket listener")
	ln, err := net.Listen("unix", fmt.Sprintf("%v/%v.sock", runnerSandbox, socket))
	if err != nil {
		logrus.Errorf("unable to create socket: %v")
		return nil, err
	}

	c := make(chan *JobEvent, 20)
	//	go testFunc(runnerSandbox, socket)

	go func() {
		defer ln.Close()
		for {
			con, err := ln.Accept()
			if err != nil {
				logrus.Errorf("unable to connect to the socket- %v", err)
				return
			}
			d := make(chan struct{})
			go handleConnection(c, con, d)
			select {
			case <-d:
				logrus.Infof("done")
				return
			case <-time.After(10 * time.Minute):
				logrus.Infof("Timed out waiting for reading of connection.")
				return
			}
		}
	}()

	return &Watcher{
		// Create a buffered channel.
		JobEvents:     make(chan *JobEvent, 20),
		runnerSandbox: runnerSandbox,
		socket:        socket,
	}, nil
}

func handleConnection(c chan *JobEvent, con net.Conn, done chan struct{}) {
	for {
		r := bufio.NewReader(con)
		l, err := r.ReadString('\n')
		if err != nil {
			logrus.Errorf("uanble to read the socket - %v", err)
			return
		}
		if strings.TrimSpace(l) == SocketEndString {
			done <- struct{}{}
			logrus.Infof("received the last event")
			return
		}
		m := map[string]interface{}{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			logrus.Infof("l - %v err - %v", l, err)
			continue
		}
		logrus.Infof("!!!!!!!!!!_________ %v------!!!!!!!!", m)
	}
}

func testFunc(runnerSandbox, identifier string) {
	c, err := net.Dial("unix", fmt.Sprintf("%v/%v.sock", runnerSandbox, identifier))
	if err != nil {
		logrus.Errorf("err- %v", err)
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	num := 0
	for {
		select {
		case <-ticker.C:
			eventFiles, err := ioutil.ReadDir(fmt.Sprintf("%v/artifacts/%v/job_events", runnerSandbox, identifier))
			if err != nil {
				logrus.Errorf("!!!!!err: %v", err)
			}

			sort.Sort(fileInfos(eventFiles))
			for _, f := range eventFiles {
				// get the number of the event.
				i, err := strconv.Atoi(strings.Split(f.Name(), "-")[0])
				if err != nil {
					logrus.Errorf("err: %v", err)
				}

				if i > num {
					d, err := ioutil.ReadFile(fmt.Sprintf("%v/artifacts/%v/job_events/%v", runnerSandbox, identifier, f.Name()))
					if err != nil {
						logrus.Errorf("unable to get job event file: %v and read the contents- %v", f.Name(), err)
						continue
					}
					je := &JobEvent{}
					err = json.Unmarshal(d, je)
					if err != nil {
						logrus.Errorf("unable to get job event file: %v and read the contents- %v", f.Name(), err)
						continue
					}
					b, _ := json.Marshal(je)
					_, err = c.Write(b)
					if err != nil {
						logrus.Errorf("unable to get job event file: %v and read the contents- %v", f.Name(), err)
						continue
					}
					num = i
					if je.Event == "playbook_on_stats" {
						c.Write([]byte(SocketEndString))
						return
					}
				}
			}
		}
	}
}
