package controller

import (
	"flag"
	"strings"

	"github.com/operator-framework/operator-sdk/pkg/ansible/events"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

const (
	runnerEventNothing    = "nothing"
	runnerEventEverything = "everything"
	runnnerEventTasks     = "tasks"
)

// Flags contains the values for the command line flags.
// You can use flags by applying them to the Controller Options.
type Flags struct {
	RunnerEventLevel string
}

// ApplyToOptions applies the flags values to the options
func (f *Flags) ApplyToOptions(o *Options) {
	switch strings.ToLower(f.RunnerEventLevel) {
	case runnerEventEverything:
		o.LoggingLevel = events.Everything
	case runnerEventNothing:
		o.LoggingLevel = events.Nothing
	case runnnerEventTasks:
		o.LoggingLevel = events.Tasks
	default:
		logrus.Warnf("unknown option for runner-event-level %v defaulting to tasks", f.RunnerEventLevel)
		o.LoggingLevel = events.Tasks
	}
}

// AddPflags - Adds flags to the pflag FlagSet returns the Flags
// which will contain the values for the flags.
func AddPflags(fs *pflag.FlagSet) *Flags {
	f := &Flags{}
	fs.StringVarP(&f.RunnerEventLevel, "runner-event-level", "r", "tasks", "The level for the ansible operator event logging options are tasks, everyting, nothing")
	return f
}

// AddGoFlags - Adds flags to the pflag FlagSet returns the Flags
// which will contain the values for the flags.
func AddGoFlags(fs *flag.FlagSet) *Flags {
	f := &Flags{}
	fs.StringVar(&f.RunnerEventLevel, "runner-event-level", "tasks", "The level for the ansible operator event logging options are tasks, everyting, nothing")
	return f
}
