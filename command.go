package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

type Command interface {
	SetStdout(io.Writer)
	StdoutPipe() (io.ReadCloser, error)
	Start(*Shell) error
	Wait() error
	Output(*Shell) ([]byte, error)
	String() string
	ExitCode() int
}

type ExecCmd struct {
	*exec.Cmd
}

func NewExecCmd(cmd *exec.Cmd) *ExecCmd {
	return &ExecCmd{cmd}
}

var _ Command = (*ExecCmd)(nil)

func (c *ExecCmd) Start(sh *Shell) error {
	return c.Cmd.Start()
}

func (c *ExecCmd) SetStdout(w io.Writer) {
	c.Cmd.Stdout = w
}

func (c *ExecCmd) Output(sh *Shell) ([]byte, error) {
	return c.Cmd.Output()
}

func (c *ExecCmd) ExitCode() int {
	return c.ProcessState.ExitCode()
}

//
// Command Pipe
//

type CommandPipe struct {
	left, right Command
}

var _ Command = &CommandPipe{}

func NewCommandPipe(left, right Command) *CommandPipe {
	return &CommandPipe{left, right}
}

func (p *CommandPipe) SetStdout(w io.Writer) {
	p.right.SetStdout(w)
}

func (p *CommandPipe) StdoutPipe() (io.ReadCloser, error) {
	return p.right.StdoutPipe()
}

func (p *CommandPipe) Start(sh *Shell) error {
	err := p.left.Start(sh)
	if err == nil {
		err = p.right.Start(sh)
	}
	return err
}

func (p *CommandPipe) Wait() error {
	return AggregateErrors(p.right.Wait(), p.left.Wait())
}

func (p *CommandPipe) ExitCode() int {
	return p.right.ExitCode()
}

func (p *CommandPipe) Output(sh *Shell) ([]byte, error) {
	// log.Print("X")
	err := p.left.Start(sh)
	if err != nil {
		return nil, err
	}
	out, err := p.right.Output(sh)
	// log.Print("Y")
	err = AggregateErrors(err, p.left.Wait())
	// log.Print("Z")
	return out, err
}

func (p *CommandPipe) String() string {
	return fmt.Sprintf("%s | %s", p.left, p.right)
}

//
// CommandSeq
//

type SeqType uint8

const (
	UncondSeq SeqType = iota
	AndSeq
	OrSeq
)

type CommandSeq struct {
	first, second Command
	errCh         chan error
	seqType       SeqType
	exitCode      int
}

var _ Command = &CommandSeq{}

func NewCommandSeq(first, second Command, tp SeqType) *CommandSeq {
	return &CommandSeq{
		first:   first,
		second:  second,
		seqType: tp,
		errCh:   make(chan error),
	}
}

func (s *CommandSeq) SetStdout(w io.Writer) {
	s.first.SetStdout(w)
	s.second.SetStdout(w)
}

func (s *CommandSeq) StdoutPipe() (io.ReadCloser, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	s.SetStdout(w)
	return r, nil
}

func (s *CommandSeq) Start(sh *Shell) error {
	err := s.first.Start(sh)
	if err != nil {
		return err
	}
	go func() {
		err := s.first.Wait()
		exitCode := s.first.ExitCode()
		var shouldStartSecond bool
		switch s.seqType {
		case UncondSeq:
			shouldStartSecond = true
		case AndSeq:
			shouldStartSecond = exitCode == 0
		case OrSeq:
			shouldStartSecond = exitCode != 0
		default:
			panic("bug!")
		}
		if shouldStartSecond {
			err = s.second.Start(sh)
			if err == nil {
				err = s.second.Wait()
				exitCode = s.second.ExitCode()
			}
		}
		s.exitCode = exitCode
		s.errCh <- err
	}()
	return nil
}

func (s *CommandSeq) Wait() error {
	return <-s.errCh
	// TODO: close the pipe?
}

func (s *CommandSeq) Output(sh *Shell) ([]byte, error) {
	out, err := s.first.Output(sh)
	if err != nil {
		return nil, err
	}
	out2, err := s.second.Output(sh)
	if err != nil {
		return nil, err
	}
	return append(out, out2...), nil
}

func (s *CommandSeq) String() string {
	return fmt.Sprintf("%s; %s", s.first, s.second)
}

func (s *CommandSeq) ExitCode() int {
	return s.exitCode
}

//
// AsyncCmd
//

type AsyncCmd struct {
	cmd Command
}

var _ Command = &AsyncCmd{}

func (c *AsyncCmd) SetStdout(w io.Writer) {
	c.cmd.SetStdout(w)
}

func (c *AsyncCmd) StdoutPipe() (io.ReadCloser, error) {
	return c.cmd.StdoutPipe()
}

func (c *AsyncCmd) Start(sh *Shell) error {
	err := c.cmd.Start(sh)
	if err != nil {
		return err
	}
	job := sh.StartJob(c)
	go func() {
		c.cmd.Wait()
		sh.StopJob(job)
	}()
	return nil
}

func (c *AsyncCmd) Wait() error {
	return nil
}

func (c *AsyncCmd) Output(sh *Shell) ([]byte, error) {
	return nil, nil
}

func (c *AsyncCmd) String() string {
	return fmt.Sprintf("%s &", c.cmd)
}

func (c *AsyncCmd) ExitCode() int {
	return 0
}

//
//
//

type Builtin interface {
	StartBuiltin(sh *Shell) error
}

type UnimplementedCommand struct {
}

var _ Command = &UnimplementedCommand{}

func (b *UnimplementedCommand) SetStdout(w io.Writer) {
}

func (b *UnimplementedCommand) StdoutPipe() (io.ReadCloser, error) {
	return nil, nil
}

func (b *UnimplementedCommand) Start(sh *Shell) error {
	return nil
}

func (b *UnimplementedCommand) Wait() error {
	return nil
}

func (b *UnimplementedCommand) Output(sh *Shell) ([]byte, error) {
	return nil, nil
}

func (b *UnimplementedCommand) String() string {
	return "unimplemented command"
}

func (b *UnimplementedCommand) ExitCode() int {
	return 0
}

type Assign struct {
	UnimplementedCommand
	items []struct{ key, value string }
}

func (a *Assign) Add(key, value string) {
	a.items = append(a.items, struct {
		key   string
		value string
	}{key, value})
}

func (a *Assign) Start(sh *Shell) error {
	for _, item := range a.items {
		sh.SetVar(item.key, item.value)
	}
	return nil
}

type Cd struct {
	UnimplementedCommand
	dir string
}

func NewCd(dir string) *Cd {
	return &Cd{dir: dir}
}

func (c *Cd) Start(sh *Shell) error {
	return sh.SetCwd(c.dir)
}

type Exit struct {
	UnimplementedCommand
	code int
}

func NewExit(code int) *Exit {
	return &Exit{code: code}
}

func (c *Exit) Start(sh *Shell) error {
	sh.Exit(c.code)
	return nil
}

func (c *Exit) ExitCode() int {
	return c.code
}

type AggregateError struct {
	err1, err2 error
}

func (e AggregateError) Error() string {
	return fmt.Sprintf("%s; %s", e.err1, e.err2)
}

func AggregateErrors(err1, err2 error) error {
	if err1 == nil {
		return err2
	}
	if err2 == nil {
		return err1
	}
	return AggregateErrors(err1, err2)
}
