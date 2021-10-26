package main

import (
	"os"
)

type Shell struct {
	globals  map[string]string
	exitCh   chan int
	exported []string
}

func NewShell() *Shell {
	return &Shell{
		globals: map[string]string{},
		exitCh:  make(chan int),
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

func (s *Shell) StartCommand(c Command) error {
	return c.Start()
}

func (s *Shell) WaitForCommand(c Command) error {
	return c.Wait()
}

func (s *Shell) Exit(code int) {
	s.exitCh <- code
}

func (s *Shell) Wait() int {
	return <-s.exitCh
}

func (s *Shell) StartJob(c Command) int {
	return 0
}

func (s *Shell) StopJob(job int) {
}

func (s *Shell) Subshell() *Shell {
	sub := NewShell()
	for k, v := range s.globals {
		sub.SetVar(k, v)
	}
	return sub
}
