package runner

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	host = "localhost"
)

// Status - POC of the for the resource status.
type Status struct {
	Ok               int       `json:"ok"`
	Changed          int       `json:"changed"`
	Skipped          int       `json:"skipped"`
	Failures         int       `json:"failures"`
	TimeOfCompletion EventTime `json:"completion"`
}

// NewStatusFromStatusJobEvent - get a POC status from a Status Job Event.
func NewStatusFromStatusJobEvent(je *StatusJobEvent) Status {
	// ok events.
	o := 0
	changed := 0
	skipped := 0
	failures := 0
	if v, ok := je.EventData.Changed[host]; ok {
		changed = v
	}
	if v, ok := je.EventData.Ok[host]; ok {
		o = v
	}
	if v, ok := je.EventData.Skipped[host]; ok {
		skipped = v
	}
	if v, ok := je.EventData.Failures[host]; ok {
		failures = v
	}
	return Status{
		Ok:               o,
		Changed:          changed,
		Skipped:          skipped,
		Failures:         failures,
		TimeOfCompletion: je.Created,
	}
}

// IsStatusEqual - determine if the POC status is equal.
func IsStatusEqual(s1, s2 Status) bool {
	return (s1.Ok == s2.Ok && s1.Changed == s2.Changed && s1.Skipped == s2.Skipped && s1.Failures == s2.Failures)
}

// NewStatusFromMap - get a POC Status from a map.
func NewStatusFromMap(sm map[string]interface{}) Status {
	//Create Old top level status
	// ok events.
	o := 0
	changed := 0
	skipped := 0
	failures := 0
	e := EventTime{}
	if v, ok := sm["changed"]; ok {
		changed = int(v.(int64))
	}
	if v, ok := sm["ok"]; ok {
		o = int(v.(int64))
	}
	if v, ok := sm["skipped"]; ok {
		skipped = int(v.(int64))
	}
	if v, ok := sm["failures"]; ok {
		failures = int(v.(int64))
	}
	if v, ok := sm["completion"]; ok {
		s := v.(string)
		e.UnmarshalJSON([]byte(s))
	}
	return Status{
		Ok:               o,
		Changed:          changed,
		Skipped:          skipped,
		Failures:         failures,
		TimeOfCompletion: e,
	}
}

// ResourceStatus -  POC status for the k8s resource.
type ResourceStatus struct {
	Status         `json:",inline"`
	FailureMessage string   `json:"reason,omitempty"`
	History        []Status `json:"history,omitempty"`
}

// UpdateResourceStatus - POC for the updated resource status.
func UpdateResourceStatus(sm map[string]interface{}, je *StatusJobEvent) (bool, ResourceStatus) {
	newStatus := NewStatusFromStatusJobEvent(je)
	oldStatus := NewStatusFromMap(sm)
	// Don't update the status if new status and old status are equal.
	if IsStatusEqual(newStatus, oldStatus) {
		return false, ResourceStatus{}
	}

	history := []Status{}
	h, ok := sm["history"]
	if ok {
		hi := h.([]interface{})
		for _, m := range hi {
			ma := m.(map[string]interface{})
			history = append(history, NewStatusFromMap(ma))
		}
	}
	history = append(history, oldStatus)
	return true, ResourceStatus{
		Status:  newStatus,
		History: history,
	}
}

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

//RunningJob - job that is currently running.
type RunningJob struct {
	WaitGroup sync.WaitGroup
	Status    chan JobEvent
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
