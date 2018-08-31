package events

import (
	"github.com/sirupsen/logrus"
	"github.com/water-hole/ansible-operator/pkg/runner/eventapi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type LogLevel int

const (
	Everything LogLevel = iota
	Tasks
	Nothing
)

type EventHandler interface {
	Handle(*unstructured.Unstructured, eventapi.JobEvent) error
}

type loggingEventHandler struct {
	LogLevel LogLevel
}

func (l loggingEventHandler) Handle(u *unstructured.Unstructured, e eventapi.JobEvent) error {
	log := logrus.WithFields(logrus.Fields{
		"component":  "logging_event_handler",
		"name":       u.GetName(),
		"namespace":  u.GetNamespace(),
		"gvk":        u.GroupVersionKind().String(),
		"event_type": e.Event,
	})
	switch l.LogLevel {
	case Everything:
		log.Infof("event: %#v")
	case Tasks:
		if t, ok := e.EventData["task"]; ok {
			log.Infof("task: %v", t)
		}
	}
	return nil
}

func NewLoggingEventHandler(l LogLevel) EventHandler {
	return loggingEventHandler{
		LogLevel: l,
	}
}
