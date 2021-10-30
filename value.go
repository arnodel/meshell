package main

import (
	"bytes"
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
		panic("not implemented")
	case '#':
		return []string{strconv.Itoa(sh.ArgCount())}, nil
	case '@':
		return sh.GetArgs(), nil
	case '$':
		panic("not implemented")
	default:
		panic("bug!")
	}
}

func (d SpecialVarValueDef) Value(sh *Shell, std StdStreams) (string, error) {
	switch d.Name {
	case '?':
		panic("not implemented")
	case '#':
		return strconv.Itoa(sh.ArgCount()), nil
	case '@':
		return strings.Join(sh.GetArgs(), " "), nil
	case '$':
		panic("not implemented")
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
