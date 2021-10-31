package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type StdStreams struct {
	In       io.Reader
	Out, Err io.Writer
}

type Command interface {
	StartJob(*Shell, StdStreams) (RunningJob, error)
	// String() string
}

type RunningJob interface {
	Wait() JobOutcome
	String() string
}

type JobOutcome struct {
	ExitCode int
	Err      error
}

func (r JobOutcome) Error() string {
	if r.Err != nil {
		return fmt.Sprintf("code %d: %s", r.ExitCode, r.Err)
	}
	return fmt.Sprintf("code %d", r.ExitCode)
}

func (r JobOutcome) Success() bool {
	return r.ExitCode == 0
}

func errorOutcome(err error) JobOutcome {
	return JobOutcome{
		ExitCode: 1,
		Err:      err,
	}
}

type AssignDef struct {
	Name string
	Val  ValueDef
}

//
// Simple Command
//

type SimpleCommand struct {
	CmdName ValueDef
	Args    []ValueDef
	Assigns []AssignDef
}

var _ Command = (*SimpleCommand)(nil)

func (d *SimpleCommand) StartJob(sh *Shell, std StdStreams) (RunningJob, error) {
	var env []string
	if len(d.Assigns) > 0 {
		env = os.Environ()
		for _, varDef := range d.Assigns {
			val, err := varDef.Val.Value(sh, std)
			if err != nil {
				return nil, err
			}
			env = append(env, fmt.Sprintf("%s=%s", varDef.Name, val))
		}
	}
	cmdName, err := d.CmdName.Value(sh, std)
	if err != nil {
		return nil, err
	}
	var args []string
	for _, valDef := range d.Args {
		chunk, err := valDef.Values(sh, std)
		if err != nil {
			return nil, err
		}
		args = append(args, chunk...)
	}
	if cmd := sh.GetFunction(cmdName); cmd != nil {
		return CallFunction(sh, std, cmd, cmdName, args)
	}
	if f := builtins[cmdName]; f != nil {
		return f(sh, std, args)
	}
	cmdPath, err := LookPath(sh.GetVar("PATH"), sh.GetCwd(), cmdName)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(cmdPath, args...)
	cmd.Dir = sh.GetCwd()
	cmd.Stdin = std.In
	cmd.Stdout = std.Out
	cmd.Stderr = std.Err
	cmd.Env = env
	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	return &ExecJob{cmd: cmd}, nil
}

type ExecJob struct {
	cmd *exec.Cmd
}

var _ RunningJob = (*ExecJob)(nil)

func (j *ExecJob) Wait() JobOutcome {
	err := j.cmd.Wait()
	if err != nil {
		return errorOutcome(err)
	}
	return JobOutcome{
		ExitCode: j.cmd.ProcessState.ExitCode(),
	}
}

func (j *ExecJob) String() string {
	return j.cmd.String()
}

type SetVarsCommand struct {
	Assigns []AssignDef
}

var _ Command = (*SetVarsCommand)(nil)

func (d *SetVarsCommand) StartJob(sh *Shell, std StdStreams) (RunningJob, error) {
	for _, varDef := range d.Assigns {
		val, err := varDef.Val.Value(sh, std)
		if err != nil {
			return nil, err
		}
		sh.SetVar(varDef.Name, val)
	}
	return &ImmediateRunningJob{name: "setvars"}, nil
}

const (
	RM_Read int = iota
	RM_Truncate
	RM_Append
	RM_ReadWrite
)

type RedirectCommand struct {
	FD          int      // File descriptor to redirect
	Replacement ValueDef // Replacement (file name or fd)
	Mode        int      // Mode to open file in
	Cmd         Command  // Command to run
	Ref         bool     // True if expecting an fd
}

var _ Command = (*RedirectCommand)(nil)

func (d *RedirectCommand) StartJob(sh *Shell, std StdStreams) (RunningJob, error) {
	repl, err := d.Replacement.Value(sh, std)
	if err != nil {
		return nil, err
	}
	var f *os.File
	if d.Ref {
		var ok bool
		switch repl {
		case "0":
			f, ok = std.In.(*os.File)
		case "1":
			f, ok = std.Out.(*os.File)
		case "2":
			f, ok = std.Err.(*os.File)
		default:
			return nil, errors.New("fd must be 0, 1, 2 for now")
		}
		if !ok {
			return nil, fmt.Errorf("fd%s is not a file", repl)
		}
	} else {
		switch d.Mode {
		case RM_Read:
			f, err = os.Open(repl)
		case RM_Truncate:
			f, err = os.Create(repl)
		case RM_Append:
			f, err = os.OpenFile(repl, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		case RM_ReadWrite:
			f, err = os.OpenFile(repl, os.O_CREATE|os.O_APPEND, 0666)
		}
		if err != nil {
			return nil, err
		}
	}
	switch d.FD {
	case 0: // stdin
		std.In = f
	case 1:
		std.Out = f
	case 2:
		std.Err = f
	}
	job, err := d.Cmd.StartJob(sh, std)
	if err != nil {
		return nil, err
	}
	if d.Ref {
		// We don't want to close the file
		return job, nil
	}
	return &RedirectJob{
		job:  job,
		file: f,
	}, nil
}

type RedirectJob struct {
	job  RunningJob
	file *os.File
}

var _ RunningJob = (*RedirectJob)(nil)

func (c *RedirectJob) Wait() JobOutcome {
	defer c.file.Close()
	return c.job.Wait()
}

func (c *RedirectJob) String() string {
	return c.job.String()
}

//
// Command Pipeline
//

type PipelineCommand struct {
	Left, Right Command
}

var _ Command = (*PipelineCommand)(nil)

func (d *PipelineCommand) StartJob(sh *Shell, std StdStreams) (RunningJob, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	rstd := std
	std.Out = w
	rstd.In = r

	left, err := d.Left.StartJob(sh, std)
	if err != nil {
		return nil, err
	}
	right, err := d.Right.StartJob(sh, rstd)
	if err != nil {
		return nil, err
	}
	w.Close()
	return &PipelineJob{
		left:  left,
		right: right,
		pipeR: r,
		pipeW: w,
	}, nil
}

type PipelineJob struct {
	left, right  RunningJob
	pipeR, pipeW *os.File
}

var _ RunningJob = (*PipelineJob)(nil)

func (p *PipelineJob) Wait() JobOutcome {
	r1 := p.right.Wait()
	p.pipeR.Close()
	r2 := p.left.Wait()

	_ = r2 // TODO: handle this error (ala bash set -o pipefail)
	return r1
}

func (p *PipelineJob) String() string {
	return fmt.Sprintf("%s | %s", p.left, p.right)
}

//
// Command List
//

type SeqType uint8

const (
	UncondSeq SeqType = iota
	AndSeq
	OrSeq
)

type CommandSequence struct {
	Left, Right Command
	SeqType     SeqType
}

var _ Command = (*CommandSequence)(nil)

func (d *CommandSequence) StartJob(sh *Shell, std StdStreams) (RunningJob, error) {
	left, err := d.Left.StartJob(sh, std)
	if err != nil {
		return nil, err
	}
	resCh := make(chan JobOutcome)
	go func() {
		res := left.Wait()
		var shouldStartSecond bool
		if !sh.ShouldStop() {
			switch d.SeqType {
			case UncondSeq:
				shouldStartSecond = true
			case AndSeq:
				shouldStartSecond = res.ExitCode == 0
			case OrSeq:
				shouldStartSecond = res.ExitCode != 0
			default:
				panic("bug!")
			}
		}
		if shouldStartSecond {
			var right RunningJob
			right, err = d.Right.StartJob(sh, std)
			if err != nil {
				res = errorOutcome(err)
			} else {
				res = right.Wait()
			}
		}
		resCh <- res
	}()
	return &JobSequence{resCh: resCh}, nil
}

type JobSequence struct {
	resCh chan JobOutcome
}

var _ RunningJob = (*JobSequence)(nil)

func (s *JobSequence) Wait() JobOutcome {
	return <-s.resCh
}

func (s *JobSequence) String() string {
	return "seqcmd"
}

//
// Background Command
//

type BackgroundCommand struct {
	Cmd Command
}

var _ Command = (*BackgroundCommand)(nil)

func (d *BackgroundCommand) StartJob(sh *Shell, std StdStreams) (RunningJob, error) {
	job, err := d.Cmd.StartJob(sh, std)
	if err != nil {
		return nil, err
	}
	// job := sh.StartJob(c)
	go func() {
		job.Wait()
		// sh.StopJob(job)
	}()
	return &BackgroundJob{job: job}, nil
}

type BackgroundJob struct {
	job RunningJob
}

var _ RunningJob = (*BackgroundJob)(nil)

func (c *BackgroundJob) Wait() JobOutcome {
	return JobOutcome{}
}

func (c *BackgroundJob) String() string {
	return ""
}

//
// Subshell
//

type SubshellCommand struct {
	Body Command
}

var _ Command = (*SubshellCommand)(nil)

func (d *SubshellCommand) StartJob(sh *Shell, std StdStreams) (RunningJob, error) {
	subshell := sh.Subshell()
	job, err := d.Body.StartJob(subshell, std)
	if err != nil {
		return nil, err
	}
	go func() {
		subshell.Exit(job.Wait().ExitCode)
	}()
	return &SubshellJob{
		subshell: subshell,
	}, nil
}

type SubshellJob struct {
	subshell *Shell
}

var _ RunningJob = &SubshellJob{}

func (c *SubshellJob) Wait() JobOutcome {
	return JobOutcome{ExitCode: c.subshell.Wait()}
}

func (c *SubshellJob) String() string {
	return "subshell"
}

type IfCommand struct {
	Condition  Command
	Then, Else Command
}

var _ Command = (*IfCommand)(nil)

func (c *IfCommand) StartJob(sh *Shell, std StdStreams) (RunningJob, error) {
	job, err := c.Condition.StartJob(sh, std)
	if err != nil {
		return nil, err
	}
	resCh := make(chan JobOutcome)
	go func() {
		res := job.Wait()
		var condJob RunningJob
		if res.Success() {
			condJob, err = c.Then.StartJob(sh, std)
		} else if c.Else != nil {
			condJob, err = c.Else.StartJob(sh, std)
		}
		if err != nil {
			resCh <- errorOutcome(err)
		} else if condJob != nil {
			resCh <- condJob.Wait()
		} else {
			resCh <- JobOutcome{}
		}
	}()
	return &JobSequence{resCh: resCh}, nil
}

type WhileCommand struct {
	Condition Command
	Body      Command
}

var _ Command = (*WhileCommand)(nil)

func (c *WhileCommand) StartJob(sh *Shell, std StdStreams) (RunningJob, error) {
	resCh := make(chan JobOutcome)
	go func() {
		var res JobOutcome
		for !sh.ShouldStop() {
			job, err := c.Condition.StartJob(sh, std)
			if err != nil {
				res = errorOutcome(err)
				break
			}
			res = job.Wait()
			if !res.Success() {
				res = JobOutcome{}
				break
			}
			job, err = c.Body.StartJob(sh, std)
			if err != nil {
				res = errorOutcome(err)
				break
			}
			job.Wait()
		}
		resCh <- res
	}()
	return &JobSequence{resCh: resCh}, nil
}

type FunctionDefCommand struct {
	Name ValueDef
	Body Command
}

var _ Command = (*FunctionDefCommand)(nil)

func (c *FunctionDefCommand) StartJob(sh *Shell, std StdStreams) (RunningJob, error) {
	name, err := c.Name.Value(sh, std)
	if err != nil {
		return nil, err
	}
	sh.SetFunction(name, c.Body)
	return &ImmediateRunningJob{name: "define function"}, nil
}

func CallFunction(sh *Shell, std StdStreams, f Command, fname string, args []string) (RunningJob, error) {
	sh.PushFrame(fname, args)
	fjob, err := f.StartJob(sh, std)
	if err != nil {
		sh.PopFrame()
		return nil, err
	}
	resCh := make(chan JobOutcome)
	go func() {
		res := fjob.Wait()
		code, returned := sh.PopFrame()
		if returned {
			resCh <- JobOutcome{ExitCode: code}
		} else {
			resCh <- res
		}
	}()
	return &JobSequence{resCh: resCh}, nil
}

//
// Bultins
//

type ImmediateRunningJob struct {
	name    string
	outcome JobOutcome
}

var _ RunningJob = &ImmediateRunningJob{}

func (b *ImmediateRunningJob) Wait() JobOutcome {
	return b.outcome
}

func (b *ImmediateRunningJob) String() string {
	return b.name
}

type SetVarsCmd struct {
	ImmediateRunningJob
	items []struct{ key, value string }
	shell *Shell
}

func (a *SetVarsCmd) Add(key, value string) {
	a.items = append(a.items, struct {
		key   string
		value string
	}{key, value})
}

func (a *SetVarsCmd) Start() error {
	for _, item := range a.items {
		a.shell.SetVar(item.key, item.value)
	}
	return nil
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
