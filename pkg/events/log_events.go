package events

import (
	"github.com/sirupsen/logrus"
	"github.com/water-hole/ansible-operator/pkg/runner/eventapi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// LogLevel - Levelt for the logging to take place.
type LogLevel int

const (
	// Tasks - only log the high level tasks.
	Tasks LogLevel = iota

	// Everything - log every event.
	Everything

	// Nothing -  this will log nothing.
	Nothing

	// Ansible Events
	EventPlaybookOnTaskStart = "playbook_on_task_start"
	EventRunnerOnOk          = "runner_on_ok"
	EventRunnerOnFailed      = "runner_on_failed"

	// Ansible Task Actions
	TaskActionSetFact = "set_fact"
	TaskActionDebug   = "debug"
)

// EventHandler - knows how to handle job events.
type EventHandler interface {
	Handle(*unstructured.Unstructured, eventapi.JobEvent)
}

type loggingEventHandler struct {
	LogLevel LogLevel
}

func (l loggingEventHandler) Handle(u *unstructured.Unstructured, e eventapi.JobEvent) {
	log := logrus.WithFields(logrus.Fields{
		"component":  "logging_event_handler",
		"name":       u.GetName(),
		"namespace":  u.GetNamespace(),
		"gvk":        u.GroupVersionKind().String(),
		"event_type": e.Event,
	})

	if l.LogLevel == Nothing {
		return
	}

	// log only the following for the 'Tasks' LogLevel
	t, ok := e.EventData["task"]
	if ok {
		setFactAction := e.EventData["task_action"] == TaskActionSetFact
		debugAction   := e.EventData["task_action"] == TaskActionDebug

		if e.Event == EventPlaybookOnTaskStart && !setFactAction && !debugAction {
			log.Infof("[playbook task]: %s", e.EventData["name"])
			return
		}
		if e.Event == EventRunnerOnOk && debugAction {
			log.Infof("[playbook debug]: %v", e.EventData["task_args"])
			return
		}
		if e.Event == EventRunnerOnFailed {
			log.Errorf("[failed]: [playbook task] '%s' failed with task_args - %v",
				t, e.EventData["task_args"])
			return
		}
	}

	// log everything else for the 'Everything' LogLevel
	if l.LogLevel == Everything {
		log.Infof("event: %#v", e.EventData)
	}
}

// NewLoggingEventHandler - Creates a Logging Event Handler to log events.
func NewLoggingEventHandler(l LogLevel) EventHandler {
	return loggingEventHandler{
		LogLevel: l,
	}
}
