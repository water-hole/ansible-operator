package runner

import "github.com/sirupsen/logrus"

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

func NewStatusFromMap(sm map[string]interface{}) Status {
	//Create Old top level status
	o := 0
	c := 0
	s := 0
	f := 0
	if v, ok := sm["changed"]; ok {
		c = v.(int)
	}
	if v, ok := sm["ok"]; ok {
		o = v.(int)
	}
	if v, ok := sm["skipped"]; ok {
		s = v.(int)
	}
	if v, ok := sm["failures"]; ok {
		f = v.(int)
	}
	if v, ok := sm["completion"]; ok {
		s := v.(string)
		logrus.Infof("%v", s)
	}
	return Status{
		Ok:       o,
		Changed:  c,
		Failures: f,
		Skipped:  s,
	}

}

type ResourceStatus struct {
	Status         `json:",inline"`
	FailureMessage string   `json:"reason,omitempty"`
	History        []Status `json:"history,omitempty"`
}

func UpdateResourceStatus(sm map[string]interface{}, je *StatusJobEvent) ResourceStatus {
	newStatus := NewStatusFromStatusJobEvent(je)
	oldStatus := NewStatusFromMap(sm)
	history := []Status{}
	h, ok := sm["history"]
	if ok {
		logrus.Infof("%+#v", h)
		hi := h.([]map[string]interface{})
		for _, m := range hi {
			history = append(history, NewStatusFromMap(m))
		}
	}
	history = append(history, oldStatus)
	return ResourceStatus{
		Status:  newStatus,
		History: history,
	}
}
