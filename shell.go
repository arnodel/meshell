package main

import (
	"os"
)

type Shell struct {
	globals  map[string]string
	done     chan struct{}
	exited   bool
	exitCode int
	exported []string
}

func NewShell() *Shell {
	return &Shell{
		globals: map[string]string{},
		done:    make(chan struct{}),
	}
}

func (s *Shell) GetVar(name string) string {
	val, ok := s.globals[name]
	if ok {
		return val
	}
	return os.Getenv(name)
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
	sub := NewShell()
	for k, v := range s.globals {
		sub.SetVar(k, v)
	}
	return sub
}
