package runner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Watcher - used to watch the file system
type Watcher struct {
}

// NewWatcher - Creates a watcher.
func NewWatcher() *Watcher {
	return &Watcher{}
}

// StartWatching - start watching the file system.
func (w *Watcher) StartWatching(runnerSandbox, identifier string, cancel chan struct{}) chan *JobEvent {
	// Create a buffered channel.
	c := make(chan *JobEvent, 20)

	// Start "watching the file system" which means a timer set for every 5 miliseconds will list the job_events directory.
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		num := 0
		for {
			select {
			case <-ticker.C:
				eventFiles, err := ioutil.ReadDir(fmt.Sprintf("%v/artifacts/%v/job_events", runnerSandbox, identifier))
				if err != nil {
					//logrus.Errorf("unable to get job event directory - %v", err)
					break
				}
				sort.Sort(fileInfos(eventFiles))
				largestNumber := num
				for _, f := range eventFiles {
					// get the number of the event.
					i, _ := strconv.Atoi(strings.Split(f.Name(), "-")[0])
					if i <= largestNumber {
						d, err := ioutil.ReadFile(fmt.Sprintf("%v/artifacts/%v/job_events/%v", runnerSandbox, identifier, f.Name()))
						if err != nil {
							//logrus.Errorf("unable to get job event file: %v and read the contents- %v", f.Name(), err)
							break
						}
						je := &JobEvent{}
						err = json.Unmarshal(d, je)
						if err != nil {
							//logrus.Errorf("unable to get job event file: %v and read the contents- %v", f.Name(), err)
							break
						}
						c <- je
						largestNumber = i
					}
				}
				num = largestNumber
			case <-cancel:
				ticker.Stop()
				close(c)
				return
			}
		}
	}()
	return c
}
