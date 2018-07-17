package runner

import (
	"github.com/sirupsen/logrus"
)

const (
	host = "localhost"
)

type Status struct {
	Ok               int       `json:"ok"`
	Changed          int       `json:"changed"`
	Skipped          int       `json:"skipped"`
	Failures         int       `json:"failures"`
	TimeOfCompletion EventTime `json:"completion"`
}

func NewStatusFromStatusJobEvent(je *StatusJobEvent) Status {
	o := 0
	c := 0
	s := 0
	f := 0
	if v, ok := je.EventData.Changed[host]; ok {
		c = v
	}
	if v, ok := je.EventData.Ok[host]; ok {
		o = v
	}
	if v, ok := je.EventData.Skipped[host]; ok {
		s = v
	}
	if v, ok := je.EventData.Failures[host]; ok {
		f = v
	}
	return Status{
		Ok:               o,
		Changed:          c,
		Skipped:          s,
		Failures:         f,
		TimeOfCompletion: je.Created,
	}
}

func IsStatusEqual(s1, s2 Status) bool {
	return (s1.Ok == s2.Ok && s1.Changed == s2.Changed && s1.Skipped == s2.Skipped && s1.Failures == s2.Failures)
}

func NewStatusFromMap(sm map[string]interface{}) Status {
	//Create Old top level status
	o := 0
	c := 0
	s := 0
	f := 0
	e := EventTime{}
	if v, ok := sm["changed"]; ok {
		c = int(v.(int64))
	}
	if v, ok := sm["ok"]; ok {
		o = int(v.(int64))
	}
	if v, ok := sm["skipped"]; ok {
		s = int(v.(int64))
	}
	if v, ok := sm["failures"]; ok {
		f = int(v.(int64))
	}
	if v, ok := sm["completion"]; ok {
		s := v.(string)
		e.UnmarshalJSON([]byte(s))
	}
	logrus.Infof("e: %v", e)
	return Status{
		Ok:               o,
		Changed:          c,
		Failures:         f,
		Skipped:          s,
		TimeOfCompletion: e,
	}
}

type ResourceStatus struct {
	Status         `json:",inline"`
	FailureMessage string   `json:"reason,omitempty"`
	History        []Status `json:"history,omitempty"`
}

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
