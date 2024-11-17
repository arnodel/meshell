package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ValueDef interface {
	Values(*Shell, StdStreams) ([]string, error)
	Value(*Shell, StdStreams) (string, error)
}

type LiteralValueDef struct {
	Val    string
	Expand bool
}

var _ ValueDef = LiteralValueDef{}

func (d LiteralValueDef) Values(sh *Shell, std StdStreams) ([]string, error) {
	if d.Expand {
		exp, err := filepath.Glob(d.Val)
		if err != nil {
			return nil, err
		}
		if exp != nil {
			return exp, nil
		}
	}
	return []string{d.Val}, nil
}

func (d LiteralValueDef) Value(sh *Shell, std StdStreams) (string, error) {
	return d.Val, nil
}

func ParamValueDef(param string) (ValueDef, error) {
	if len(param) == 0 {
		return nil, errors.New("invalid empty param")
	}
	switch p0 := param[0]; {
	case p0 >= '0' && p0 <= '9':
		argnum, err := strconv.ParseInt(param, 10, 64)
		if err != nil {
			return nil, err
		}
		return ArgValueDef{Number: int(argnum)}, nil
	case strings.IndexByte("?#@$", p0) != -1:
		return SpecialVarValueDef{Name: p0}, nil
	default:
		return VarValueDef{Name: param}, nil
	}
}

type VarValueDef struct {
	Name string
}

func (d VarValueDef) Values(sh *Shell, std StdStreams) ([]string, error) {
	return []string{sh.GetVar(d.Name)}, nil
}

func (d VarValueDef) Value(sh *Shell, std StdStreams) (string, error) {
	return sh.GetVar(d.Name), nil
}

type ArgValueDef struct {
	Number int
}

func (d ArgValueDef) Values(sh *Shell, std StdStreams) ([]string, error) {
	return []string{sh.GetArg(d.Number)}, nil
}

func (d ArgValueDef) Value(sh *Shell, std StdStreams) (string, error) {
	return sh.GetArg(d.Number), nil
}

type SpecialVarValueDef struct {
	Name byte
}

func (d SpecialVarValueDef) Values(sh *Shell, std StdStreams) ([]string, error) {
	switch d.Name {
	case '?':
		return []string{strconv.Itoa(sh.LastExitCode())}, nil
	case '#':
		return []string{strconv.Itoa(sh.ArgCount())}, nil
	case '@':
		return sh.GetArgs(), nil
	case '$':
		return []string{strconv.Itoa(os.Getpid())}, nil
	default:
		panic("bug!")
	}
}

func (d SpecialVarValueDef) Value(sh *Shell, std StdStreams) (string, error) {
	switch d.Name {
	case '?':
		return strconv.Itoa(sh.LastExitCode()), nil
	case '#':
		return strconv.Itoa(sh.ArgCount()), nil
	case '@':
		return strings.Join(sh.GetArgs(), " "), nil
	case '$':
		return strconv.Itoa(os.Getpid()), nil
	default:
		panic("bug!")
	}
}

type CommandValueDef struct {
	Cmd Command
}

func (d CommandValueDef) Values(sh *Shell, std StdStreams) ([]string, error) {
	v, err := d.Value(sh, std)
	if err != nil {
		return nil, err
	}
	return []string{v}, nil
}

func (d CommandValueDef) Value(sh *Shell, std StdStreams) (string, error) {
	var buf bytes.Buffer
	std.Out = &buf
	job, err := d.Cmd.StartJob(sh, std)
	if err != nil {
		return "", err
	}
	res := job.Wait()
	if res.ExitCode != 0 {
		return "", res
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

type CompositeValueDef struct {
	Parts []ValueDef
}

func (d CompositeValueDef) Values(sh *Shell, std StdStreams) ([]string, error) {
	v, err := d.Value(sh, std)
	if err != nil {
		return nil, err
	}
	return []string{v}, nil
}

func (d CompositeValueDef) Value(sh *Shell, std StdStreams) (string, error) {
	var b strings.Builder
	for _, part := range d.Parts {
		s, err := part.Value(sh, std)
		if err != nil {
			return "", err
		}
		b.WriteString(s)
	}
	return b.String(), nil
}
