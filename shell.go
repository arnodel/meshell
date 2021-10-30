package main

import (
	"fmt"
	"os"
)

type Shell struct {
	args      []string
	globals   map[string]string
	functions map[string]Command
	done      chan struct{}
	exited    bool
	exitCode  int
	exported  []string
}

func NewShell(args []string) *Shell {
	return &Shell{
		args:      args,
		globals:   map[string]string{},
		done:      make(chan struct{}),
		functions: map[string]Command{},
	}
}

func (s *Shell) GetArg(n int) string {
	if n >= len(s.args) {
		return ""
	}
	return s.args[n]
}

func (s *Shell) GetVar(name string) string {
	val, ok := s.globals[name]
	if ok {
		return val
	}
	return os.Getenv(name)
}

func (s *Shell) GetFunction(name string) Command {
	return s.functions[name]
}

func (s *Shell) Export(name string) {
	for _, gname := range s.exported {
		if name == gname {
			return
		}
	}
	s.exported = append(s.exported, name)
}

func (s *Shell) SetVar(name, val string) {
	s.globals[name] = val
}

func (s *Shell) SetFunction(name string, body Command) {
	fmt.Println("Setting function:", name)
	s.functions[name] = body
}

func (s *Shell) SetCwd(dir string) error {
	return os.Chdir(dir)
}

func (s *Shell) GetCwd() (string, error) {
	return os.Getwd()
}

func (s *Shell) Exited() bool {
	return s.exited
}

func (s *Shell) Exit(code int) {
	if !s.exited {
		s.exited = true
		s.exitCode = code
		close(s.done)
	}
}

func (s *Shell) ExitCode() int {
	return s.exitCode
}

func (s *Shell) Wait() int {
	<-s.done
	return s.exitCode
}

func (s *Shell) Subshell() *Shell {
	args := make([]string, len(s.args))
	for i, x := range s.args {
		args[i] = x
	}
	sub := NewShell(args)
	for k, v := range s.globals {
		sub.SetVar(k, v)
	}
	return sub
}
