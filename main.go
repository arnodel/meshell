package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/arnodel/grammar"
	"github.com/peterh/liner"
)

func main() {
	linr := liner.NewLiner()
	defer linr.Close()
	linr.SetCtrlCAborts(true)

	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	shell := NewShell(cwd)
	go func() {
		code := shell.Wait()
		linr.Close()
		os.Exit(code)
	}()
outerLoop:
	for {
		cwd, _ = os.Getwd()
		line, err := linr.Prompt(fmt.Sprintf("%s$ ", cwd))
		if err == io.EOF {
			fmt.Fprintln(os.Stdout, "\nBye!")
			return
		} else if err != nil {
			fmt.Println(err)
			continue
		}
		for {
			line = line + "\n"
			tokenStream, err := tokeniseCommand(line)
			if err != nil {
				fmt.Println(err)
				continue outerLoop
			}
			var parsedLine Line
			parseErr := grammar.Parse(&parsedLine, tokenStream)
			// tokenStream.Dump(os.Stdout)
			if parseErr == nil {
				linr.AppendHistory(strings.TrimSpace(line))
				if parsedLine.CmdList == nil {
					continue outerLoop
				}
				cmd, err := parsedLine.CmdList.GetCommand(shell, os.Stdin)
				if err == nil {
					cmd.SetStdout(os.Stdout)
					err = shell.StartCommand(cmd)
				}
				if err == nil {
					err = shell.WaitForCommand(cmd)
				}
				if err != nil {
					fmt.Println(err)
				}
				continue outerLoop
			} else if parseErr.Token == grammar.EOF {
				more, err := linr.Prompt("> ")
				if err == io.EOF {
					return
				} else if err != nil {
					panic(err)
				}
				line = line + more
			} else {
				fmt.Println(parseErr)
				continue outerLoop
			}
		}
	}
}

type Token = grammar.SimpleToken

// Commands

var tokeniseCommand = grammar.SimpleTokeniser([]grammar.TokenDef{
	{
		Mode: "cmd",
		Ptn:  `[ \t]+`,
	},
	{
		Mode: "cmd",
		Ptn:  `\\\n`,
	},
	{
		Mode: "cmd",
		Name: "logical",
		Ptn:  `(?:&&|\|\|)\s*`,
	},
	{
		Mode: "cmd",
		Ptn:  `[;&\n]\s*`,
		Name: "term",
	},

	{
		Mode: "cmd",
		Name: "envvar",
		Ptn:  `\$[a-zA-Z_][a-zA-Z0-9_-]*`,
	},
	{
		Mode: "cmd",
		Name: "assign",
		Ptn:  `[a-zA-Z_][a-zA-Z0-9_-]*=`,
	},
	{
		Mode:     "cmd",
		Name:     "dollarbkt",
		Ptn:      `\$\(\s*`,
		PushMode: "cmd",
	},
	{
		Mode: "cmd",
		Name: "pipe",
		Ptn:  `\|\s*`,
	},
	{
		Mode:    "cmd",
		Name:    "closebkt",
		Ptn:     `\)`,
		PopMode: true,
	},
	{
		Mode: "cmd",
		Name: "redirect",
		Ptn:  `>>|>|<<|<`,
	},
	{
		Mode:     "cmd",
		Name:     "startquote",
		Ptn:      `"`,
		PushMode: "str",
	},
	{
		Mode: "cmd",
		Name: "litstr",
		Ptn:  `'[^']*'`,
	},
	{
		Mode: "cmd",
		Name: "literal",
		Ptn:  `[^\s();&\$|]+`,
	},
	{
		Mode:    "str",
		Name:    "endquote",
		Ptn:     `"`,
		PopMode: true,
	},
	{
		Mode: "str",
		Name: "escaped",
		Ptn:  `\.`,
	},
	{
		Mode: "str",
		Name: "envvar",
		Ptn:  `\$[a-zA-Z_][a-zA-Z0-9_-]*`,
	},
	{
		Mode:     "str",
		Name:     "dollarbkt",
		Ptn:      `\$\(\s*`,
		PushMode: "cmd",
	},
	{
		Mode: "str",
		Name: "lit",
		Ptn:  `[^\\$"]+`,
	}})

type Line struct {
	grammar.Seq
	CmdList *CmdList
	EOF     Token `tok:"EOF"`
}

type CmdList struct {
	grammar.Seq
	First CmdListItem
	Rest  []CmdListItem
}

func (c *CmdList) GetCommand(sh *Shell, stdin io.ReadCloser) (Command, error) {
	cmdSeq, err := c.First.GetCommand(sh, stdin)
	if err != nil {
		return nil, err
	}
	for _, item := range c.Rest {
		cmd, err := item.GetCommand(sh, stdin)
		if err != nil {
			return nil, err
		}
		cmdSeq = NewCommandSeq(cmdSeq, cmd, UncondSeq)
	}
	return cmdSeq, nil
}

type CmdListItem struct {
	grammar.Seq
	Cmd CmdLogical
	Op  *Token `tok:"term"`
}

func (c *CmdListItem) GetCommand(sh *Shell, stdin io.ReadCloser) (Command, error) {
	cmd, err := c.Cmd.GetCommand(sh, stdin)
	if err != nil {
		return nil, err
	}
	if c.Op == nil {
		return cmd, err
	}
	switch c.Op.Value()[0] {
	case '&':
		cmd = &AsyncCmd{cmd: cmd}
	case '\n', ';':
		// Nothing to do
	default:
		panic("bug!")
	}
	return cmd, nil
}

type CmdLogical struct {
	grammar.Seq
	First Pipeline
	Rest  []NextPipeline
}

func (c *CmdLogical) GetCommand(sh *Shell, stdin io.ReadCloser) (Command, error) {
	cmdSeq, err := c.First.GetCommand(sh, stdin)
	if err != nil {
		return nil, err
	}
	for _, next := range c.Rest {
		cmd, err := next.Cmd.GetCommand(sh, stdin)
		if err != nil {
			return nil, err
		}
		var op SeqType
		switch next.Op.Value()[:2] {
		case "||":
			op = OrSeq
		case "&&":
			op = AndSeq
		default:
			panic("bug!")
		}
		cmdSeq = NewCommandSeq(cmdSeq, cmd, op)
	}
	return cmdSeq, nil
}

type NextPipeline struct {
	grammar.Seq
	Op  Token `tok:"logical"`
	Cmd Pipeline
}

type SimpleCmd struct {
	grammar.Seq
	Assignments []Assignment
	Parts       []CmdPart
}

func (c *SimpleCmd) sortParts() ([]*Value, []*Redirect) {
	var vals []*Value
	var redirects []*Redirect
	for _, part := range c.Parts {
		switch {
		case part.Value != nil:
			vals = append(vals, part.Value)
		case part.Redirect != nil:
			redirects = append(redirects, part.Redirect)
		default:
			panic("bug!")
		}
	}
	return vals, redirects
}

type CmdPart struct {
	grammar.OneOf
	Value    *Value
	Redirect *Redirect
}

type Redirect struct {
	grammar.Seq
	Op   Token `tok:"redirect"`
	File Value
}

func (c *SimpleCmd) GetCommand(sh *Shell, stdin io.ReadCloser) (Command, error) {
	args, redirects := c.sortParts()
	// TODO: deal with redirects
	_ = redirects
	if len(args) == 0 {
		assignCmd := new(Assign)

		for _, a := range c.Assignments {
			key, val, err := a.KeyValue(sh)
			if err != nil {
				return nil, err
			}
			assignCmd.Add(key, val)
		}
		return assignCmd, nil
	}
	evaledArgs := make([]string, 0, len(args))
	var err error
	for _, arg := range args {
		vals, err := arg.Eval(sh)
		if err != nil {
			return nil, err
		}
		evaledArgs = append(evaledArgs, vals...)

	}
	cmdName := evaledArgs[0]
	if err != nil {
		return nil, err

	}
	switch cmdName {
	case "cd":
		dir := ""
		switch len(args) {
		case 1:
			dir, err = os.UserHomeDir()
		case 2:
			dir = evaledArgs[1]
		default:
			err = errors.New("cd: wrong number of arguments")
		}
		if err != nil {
			return nil, err
		}
		return NewCd(dir), nil
	case "exit":
		var code int64
		switch len(args) {
		case 1:
			// default exit code
		case 2:
			codeStr := evaledArgs[1]
			code, err = strconv.ParseInt(codeStr, 10, 64)
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("exit: wrong number of arguments")
		}
		return NewExit(int(code)), nil
	}

	cmd := exec.Command(cmdName, evaledArgs[1:]...)
	wd, err := sh.GetCwd()
	if err != nil {
		return nil, err
	}
	cmd.Dir = wd
	cmd.Stdin = stdin

	if len(c.Assignments) > 0 {
		env := os.Environ()
		for _, a := range c.Assignments {
			vals, err := a.Value.Eval(sh)
			if err != nil {
				return nil, err
			}
			env = append(env, a.Dest.Value()+strings.Join(vals, " "))
		}
		cmd.Env = env
	}
	return NewExecCmd(cmd), nil
}

type AssignmentList struct {
	grammar.Seq
	First Assignment
	Rest  []Assignment
}

type Assignment struct {
	grammar.Seq
	Dest  Token `tok:"assign"`
	Value Value
}

func (a *Assignment) KeyValue(sh *Shell) (string, string, error) {
	key := strings.TrimSuffix(a.Dest.Value(), "=")
	values, err := a.Value.Eval(sh)
	if err != nil {
		return "", "", err
	}
	return key, strings.Join(values, " "), nil
}

type Pipeline struct {
	grammar.Seq
	FirstCmd SimpleCmd
	Pipes    []PipedCmd
}

func (c *Pipeline) GetCommand(sh *Shell, stdin io.ReadCloser) (Command, error) {
	cmd, err := c.FirstCmd.GetCommand(sh, stdin)
	if err != nil {
		return nil, err
	}
	for _, pipe := range c.Pipes {
		r, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		right, err := pipe.Cmd.GetCommand(sh, r)
		if err != nil {
			return nil, err
		}
		cmd = NewCommandPipe(cmd, right)
	}
	return cmd, nil
}

type PipedCmd struct {
	grammar.Seq
	Pipe Token `tok:"pipe"`
	Cmd  SimpleCmd
}

type Value struct {
	grammar.OneOf
	Literal    *Token `tok:"literal"`
	String     *String
	Quote      *Token `tok:"litstr"`
	EnvVar     *Token `tok:"envvar"`
	DollarStmt *DollarStmt
}

func (v *Value) Eval(sh *Shell) ([]string, error) {
	switch {
	case v.Literal != nil:
		val := v.Literal.Value()
		exp, err := filepath.Glob(val)
		if err != nil {
			return nil, err
		}
		if exp == nil {
			return []string{val}, nil
		}
		return exp, nil
	case v.DollarStmt != nil:
		s, err := v.DollarStmt.Eval(sh)
		if err != nil {
			return nil, err
		}
		return []string{s}, nil
	case v.EnvVar != nil:
		return []string{sh.GetVar(v.EnvVar.Value()[1:])}, nil
	case v.String != nil:
		s, err := v.String.Eval(sh)
		if err != nil {
			return nil, err
		}
		return []string{s}, nil
	case v.Quote != nil:
		return []string{strings.Trim(v.Quote.Value(), "'")}, nil
	default:
		panic("bug!")
	}
}

type DollarStmt struct {
	grammar.Seq
	Open  Token `tok:"dollarbkt"`
	Cmds  CmdList
	Close Token `tok:"closebkt"`
}

func (s *DollarStmt) Eval(sh *Shell) (string, error) {
	cmd, err := s.Cmds.GetCommand(sh, os.Stdin)
	if err != nil {
		return "", err
	}
	b, err := cmd.Output(sh)
	if err != nil {
		return "", err
	}
	b = bytes.TrimSuffix(b, []byte("\n"))
	return string(b), err
}

type String struct {
	grammar.Seq
	Open   Token `tok:"startquote"`
	Chunks []StringChunk
	Close  Token `tok:"endquote"`
}

func (s *String) Eval(sh *Shell) (string, error) {
	var b strings.Builder
	for _, chunk := range s.Chunks {
		switch {
		case chunk.Lit != nil:
			b.WriteString(chunk.Lit.Value())
		case chunk.DollarStmt != nil:
			val, err := chunk.DollarStmt.Eval(sh)
			if err != nil {
				return "", err
			}
			b.WriteString(val)
		case chunk.EnvVar != nil:
			b.WriteString(sh.GetVar(chunk.EnvVar.Value()[1:]))
		case chunk.Escaped != nil:
			r, _, _, err := strconv.UnquoteChar(chunk.Escaped.Value(), '"')
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
		default:
			panic("bug!")
		}
	}
	return b.String(), nil
}

type StringChunk struct {
	grammar.OneOf
	Lit        *Token `tok:"lit"`
	DollarStmt *DollarStmt
	EnvVar     *Token `tok:"envvar"`
	Escaped    *Token `tok:"escaped"`
}
