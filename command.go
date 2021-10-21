package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
)

type StdStreams struct {
	In       io.Reader
	Out, Err io.Writer
}

type CommandDef interface {
	Command(*Shell, StdStreams) (Command, error)
	// String() string
}

type Command interface {
	Start() error
	Wait() error
	String() string
	ExitCode() int
}

type ExecCmdDef struct {
	Parts []ValueDef
	Env   []VarDef
}

type VarDef struct {
	Name string
	Val  ValueDef
}

func (d *ExecCmdDef) Command(sh *Shell, std StdStreams) (Command, error) {
	var parts []string
	for _, valDef := range d.Parts {
		chunk, err := valDef.Values(sh, std)
		if err != nil {
			return nil, err
		}
		parts = append(parts, chunk...)
	}
	if len(parts) == 0 {
		cmd := &Assign{shell: sh}
		for _, varDef := range d.Env {
			val, err := varDef.Val.Value(sh, std)
			if err != nil {
				return nil, err
			}
			cmd.Add(varDef.Name, val)
		}
		return cmd, nil
	}
	var env []string
	if len(d.Env) > 0 {
		env = os.Environ()
		for _, varDef := range d.Env {
			val, err := varDef.Val.Value(sh, std)
			if err != nil {
				return nil, err
			}
			env = append(env, fmt.Sprintf("%s=%s", varDef.Name, val))
		}
	}
	cmdName := parts[0]
	args := parts[1:]
	var err error
	switch cmdName {
	case "cd":
		dir := ""
		switch len(args) {
		case 0:
			dir, err = os.UserHomeDir()
		case 1:
			dir = args[0]
		default:
			err = errors.New("cd: wrong number of arguments")
		}
		if err != nil {
			return nil, err
		}
		return &Cd{dir: dir, shell: sh}, nil
	case "exit":
		var code int64
		switch len(args) {
		case 0:
			// default exit code
		case 1:
			codeStr := args[0]
			code, err = strconv.ParseInt(codeStr, 10, 64)
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("exit: wrong number of arguments")
		}
		return &Exit{code: int(code), shell: sh}, nil
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	dir, err := sh.GetCwd()
	if err != nil {
		return nil, err
	}
	cmd.Dir = dir
	cmd.Stdin = std.In
	cmd.Stdout = std.Out
	cmd.Stderr = std.Err
	cmd.Env = env
	return NewExecCmd(cmd), nil
}

type ExecCmd struct {
	*exec.Cmd
}

func NewExecCmd(cmd *exec.Cmd) *ExecCmd {
	return &ExecCmd{cmd}
}

var _ Command = (*ExecCmd)(nil)

func (c *ExecCmd) ExitCode() int {
	return c.ProcessState.ExitCode()
}

//
// Command Pipe
//

type PipelineCmdDef struct {
	Left, Right CommandDef
}

func (d *PipelineCmdDef) Command(sh *Shell, std StdStreams) (Command, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	rstd := std
	std.Out = w
	rstd.In = r

	left, err := d.Left.Command(sh, std)
	if err != nil {
		return nil, err
	}
	right, err := d.Right.Command(sh, rstd)
	if err != nil {
		return nil, err
	}
	return &PipelineCmd{
		left:  left,
		right: right,
		pipeR: r,
		pipeW: w,
		errCh: make(chan error),
	}, nil
}

type PipelineCmd struct {
	left, right  Command
	pipeR, pipeW *os.File
	errCh        chan error
}

var _ Command = &PipelineCmd{}

func (p *PipelineCmd) Start() error {
	err := p.left.Start()
	if err == nil {
		err = p.right.Start()
	}
	return err
}

func (p *PipelineCmd) Wait() error {
	go func() {
		defer p.pipeW.Close()
		p.errCh <- p.left.Wait()
	}()
	defer p.pipeR.Close()
	return AggregateErrors(<-p.errCh, p.right.Wait())
}

func (p *PipelineCmd) ExitCode() int {
	return p.right.ExitCode()
}

func (p *PipelineCmd) String() string {
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

type SeqCmdDef struct {
	Left, Right CommandDef
	SeqType     SeqType
}

func (d SeqCmdDef) Command(sh *Shell, std StdStreams) (Command, error) {
	left, err := d.Left.Command(sh, std)
	if err != nil {
		return nil, err
	}
	right, err := d.Right.Command(sh, std)
	if err != nil {
		return nil, err
	}
	return &SeqCmd{
		left:    left,
		right:   right,
		seqType: d.SeqType,
		errCh:   make(chan error),
	}, nil
}

type SeqCmd struct {
	left, right Command
	errCh       chan error
	seqType     SeqType
	exitCode    int
}

var _ Command = &SeqCmd{}

func (s *SeqCmd) Start() error {
	err := s.left.Start()
	if err != nil {
		return err
	}
	go func() {
		err := s.left.Wait()
		exitCode := s.left.ExitCode()
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
			err = s.right.Start()
			if err == nil {
				err = s.right.Wait()
				exitCode = s.right.ExitCode()
			}
		}
		s.exitCode = exitCode
		s.errCh <- err
	}()
	return nil
}

func (s *SeqCmd) Wait() error {
	return <-s.errCh
	// TODO: close the pipe?
}

func (s *SeqCmd) String() string {
	return fmt.Sprintf("%s; %s", s.left, s.right)
}

func (s *SeqCmd) ExitCode() int {
	return s.exitCode
}

//
// BackgroundCmd
//

type BackgroundCmdDef struct {
	Cmd CommandDef
}

func (d BackgroundCmdDef) Command(sh *Shell, std StdStreams) (Command, error) {
	cmd, err := d.Cmd.Command(sh, std)
	if err != nil {
		return nil, err
	}
	return &BackgroundCmd{cmd: cmd}, nil
}

type BackgroundCmd struct {
	cmd Command
}

var _ Command = &BackgroundCmd{}

func (c *BackgroundCmd) Start() error {
	err := c.cmd.Start()
	if err != nil {
		return err
	}
	// job := sh.StartJob(c)
	go func() {
		c.cmd.Wait()
		// sh.StopJob(job)
	}()
	return nil
}

func (c *BackgroundCmd) Wait() error {
	return nil
}

func (c *BackgroundCmd) String() string {
	return fmt.Sprintf("%s &", c.cmd)
}

func (c *BackgroundCmd) ExitCode() int {
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

func (b *UnimplementedCommand) Start() error {
	return nil
}

func (b *UnimplementedCommand) Wait() error {
	return nil
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
	shell *Shell
}

func (a *Assign) Add(key, value string) {
	a.items = append(a.items, struct {
		key   string
		value string
	}{key, value})
}

func (a *Assign) Start() error {
	for _, item := range a.items {
		a.shell.SetVar(item.key, item.value)
	}
	return nil
}

type Cd struct {
	UnimplementedCommand
	dir   string
	shell *Shell
}

func (c *Cd) Start() error {
	return c.shell.SetCwd(c.dir)
}

type Exit struct {
	UnimplementedCommand
	code  int
	shell *Shell
}

func (c *Exit) Start() error {
	c.shell.Exit(c.code)
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
