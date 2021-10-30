package main

import (
	"errors"
	"fmt"
	"os"
)

type Shell struct {
	name      string
	args      []string
	globals   map[string]string
	functions map[string]Command
	done      chan struct{}
	exited    bool
	exitCode  int
	exported  []string
	frames    []Frame
}

type Frame struct {
	name       string
	args       []string
	locals     map[string]string
	returned   bool
	returnCode int
}

func NewShell(name string, args []string) *Shell {
	return &Shell{
		name:      name,
		args:      args,
		globals:   map[string]string{},
		done:      make(chan struct{}),
		functions: map[string]Command{},
	}
}

func (s *Shell) currentFrame() *Frame {
	n := len(s.frames)
	if n == 0 {
		return nil
	}
	return &s.frames[n-1]
}

func (s *Shell) PushFrame(name string, args []string) {
	s.frames = append(s.frames, Frame{name: name, args: args})
}

func (s *Shell) PopFrame() (int, bool) {
	f := s.currentFrame()
	if f == nil {
		panic("no frame to pop")
	}
	s.frames = s.frames[:len(s.frames)-1]
	return f.returnCode, f.returned
}

func (s *Shell) Return(code int) error {
	f := s.currentFrame()
	if f == nil {
		return errors.New("no function to return from")
	}
	f.returnCode = code
	f.returned = true
	return nil
}

func (s *Shell) Returned() bool {
	f := s.currentFrame()
	return f != nil && f.returned
}

func (s *Shell) GetArg(n int) string {
	f := s.currentFrame()
	if n == 0 {
		if f != nil {
			return f.name
		}
		return s.name
	}
	var args []string
	n--
	if f != nil {
		args = f.args
	} else {
		args = s.args
	}
	if n >= len(args) {
		return ""
	}
	return args[n]
}

func (s *Shell) ArgCount() int {
	f := s.currentFrame()
	if f != nil {
		return len(f.args)
	}
	return len(s.args)
}

func (s *Shell) GetArgs() []string {
	f := s.currentFrame()
	if f != nil {
		return f.args
	}
	return s.args
}

func (s *Shell) ShiftArgs(n int) {
	f := s.currentFrame()
	if f != nil {
		f.args = f.args[n:]
	} else {
		s.args = s.args[n:]
	}
}

func (s *Shell) GetVar(name string) string {
	var (
		val string
		ok  bool
		f   = s.currentFrame()
	)
	if f != nil {
		val, ok = f.locals[name]
	}
	if !ok {
		val, ok = s.globals[name]
	}
	if !ok {
		val = os.Getenv(name)
	}
	return val
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
	f := s.currentFrame()
	if f != nil {
		_, ok := f.locals[name]
		if ok {
			f.locals[name] = val
			return
		}
	}
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

func (s *Shell) ShouldStop() bool {
	return s.Exited() || s.Returned()
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
	sub := NewShell(s.name, args)
	for k, v := range s.globals {
		sub.SetVar(k, v)
	}
	return sub
}
