package controller

import (
	"flag"

	"github.com/operator-framework/operator-sdk/pkg/ansible/events"
	"github.com/spf13/pflag"
)

func AddPflags(fs *pflag.FlagSet) *Options {
	return &Options{}
}

type Flags struct {
	Level int
}

func (f *Flags) CreateOptions() *Options {
	return &Options{
		LoggingLevel: events.LogLevel(f.Level),
	}
}

func AddGoFlags(fs *flag.FlagSet) *Flags {
	f := &Flags{}

	fs.IntVar(&f.Level, "runner-loglevel", 0, "The level for the ansible operator event logging, 0 for tasks, 1 for everyting and >1 for nothing")
	return f
}
