package main

import "os"

type Shell struct {
	vars   map[string]string
	exitCh chan int
}

func NewShell() *Shell {
	return &Shell{
		vars:   map[string]string{},
		exitCh: make(chan int),
	}
}

func (s *Shell) GetVar(name string) string {
	val, ok := s.vars[name]
	if ok {
		return val
	}
	return os.Getenv(name)
}

func (s *Shell) SetVar(name, val string) {
	s.vars[name] = val
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
