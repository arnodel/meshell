package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/arnodel/grammar"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	shell := NewGosh(cwd)
	reader := bufio.NewReader(os.Stdin)
	for {
		cwd, _ = os.Getwd()
		fmt.Printf("%s$ ", cwd)
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			return
		} else if err != nil {
			panic(err)
		}
		tokenStream, _ := tokeniseCommand(line)
		var parsedLine Line
		parseErr := grammar.Parse(&parsedLine, tokenStream)
		if parseErr != nil {
			panic(err)
		}
		// grammar.PrettyWrite(os.Stdout, parsedLine)
		err = parsedLine.Exec(shell)
		if err != nil {
			fmt.Println(err)
		}
	}
}

type Shell struct {
	cwd  string
	vars map[string]string
}

func NewGosh(cwd string) *Shell {
	return &Shell{
		cwd:  cwd,
		vars: map[string]string{},
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
	// log.Print(c)
	return c.Start(s)
}

func (s *Shell) WaitForCommand(c Command) error {
	return c.Wait()
}

type Token = grammar.SimpleToken

// Commands
func getStringToken(s string) string {
	last := s[0]
	depth := 0
	if last != '"' {
		return ""
	}
	for i := 1; i < len(s); i++ {
		switch last {
		case '\\':
			last = 0
			continue
		case '$':
			if s[i] == '(' {
				last = 0
				depth++
				continue
			}
		}
		switch s[i] {
		case '"':
			if depth == 0 {
				return s[:i+1]
			}
		case ')':
			depth--
		}
		last = s[i]
	}
	return ""
}

var tokeniseCommand = grammar.SimpleTokeniser([]grammar.TokenDef{
	{
		Ptn: `\s+`,
	},
	{
		Name: "envvar",
		Ptn:  `\$[a-zA-Z_][a-zA-Z0-9_-]*`,
	},
	{
		Name: "assign",
		Ptn:  `[a-zA-Z_][a-zA-Z0-9_-]*=`,
	},
	{
		Name: "dollarbkt",
		Ptn:  `\$\(`,
	},
	{
		Name: "op",
		Ptn:  `[&()|;]`,
	},
	{
		Name:    "string",
		Ptn:     `".`,
		Special: getStringToken,
	},
	{
		Name: "quote",
		Ptn:  `'[^']*'`,
	},
	{
		Name: "literal",
		Ptn:  `[^\s();&$]+`,
	},
})

type Line struct {
	grammar.Seq
	Stmts []Stmt
	Amp   *Token `tok:"op,&"`
}

func (l *Line) Exec(sh *Shell) error {
	for i, stmt := range l.Stmts {
		if err := stmt.Exec(sh, l.Amp != nil && i == len(l.Stmts)-1); err != nil {
			return err
		}
	}
	return nil
}

type Stmt struct {
	grammar.OneOf
	Cmd         *PipedCmd
	Assignments *AssignmentList
}

func (s *Stmt) Exec(sh *Shell, bgnd bool) error {
	var cmd Command
	var err error
	switch {
	case s.Cmd != nil:
		cmd, err = s.Cmd.GetCommand(sh, os.Stdin)
		if err != nil {
			return err
		}
	case s.Assignments != nil:
		assignCmd := new(Assign)
		key, val, err := s.Assignments.First.KeyValue(sh)
		if err != nil {
			return err
		}
		assignCmd.Add(key, val)
		for _, a := range s.Assignments.Rest {
			key, val, err = a.KeyValue(sh)
			if err != nil {
				return err
			}
			assignCmd.Add(key, val)
		}
		cmd = assignCmd
	default:
		panic("bug!")
	}
	cmd.SetStdout(os.Stdout)
	sh.StartCommand(cmd)
	if bgnd {
		go sh.WaitForCommand(cmd)
		return nil
	} else {
		return sh.WaitForCommand(cmd)
	}
}

type Cmd struct {
	grammar.Seq
	Assignments []Assignment
	CmdName     Value
	Parts       []Value
}

func (c *Cmd) GetCommand(sh *Shell, stdin io.ReadCloser) (Command, error) {
	cmdName, err := c.CmdName.Eval(sh)
	if err != nil {
		return nil, err
	}
	switch cmdName {
	case "cd":
		dir := ""
		switch len(c.Parts) {
		case 0:
			dir, err = os.UserHomeDir()
		case 1:
			dir, err = c.Parts[0].Eval(sh)
		default:
			err = errors.New("cd: wrong number of arguments")
		}
		if err != nil {
			return nil, err
		}
		return NewCd(dir), nil
	}
	args := make([]string, len(c.Parts))
	for i, p := range c.Parts {
		arg, err := p.Eval(sh)
		if err != nil {
			return nil, err
		}
		args[i] = arg
	}
	cmd := exec.Command(cmdName, args...)
	wd, err := sh.GetCwd()
	if err != nil {
		return nil, err
	}
	cmd.Dir = wd
	cmd.Stdin = stdin

	if len(c.Assignments) > 0 {
		env := os.Environ()
		for _, a := range c.Assignments {
			val, err := a.Value.Eval(sh)
			if err != nil {
				return nil, err
			}
			env = append(env, a.Dest.Value()+val)
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
	value, err := a.Value.Eval(sh)
	if err != nil {
		return "", "", err
	}
	return key, value, nil
}

type PipedCmd struct {
	grammar.Seq
	FirstCmd Cmd
	Pipes    []Pipe
	End      *Token `tok:"op,;"`
}

func (c *PipedCmd) GetCommand(sh *Shell, stdin io.ReadCloser) (Command, error) {
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

type Pipe struct {
	grammar.Seq
	Pipe Token `tok:"op,|"`
	Cmd  Cmd
}

type Value struct {
	grammar.OneOf
	Literal    *Token `tok:"literal"`
	String     *Token `tok:"string"`
	Quote      *Token `tok:"quote"`
	EnvVar     *Token `tok:"envvar"`
	DollarStmt *DollarStmt
}

func (v *Value) Eval(sh *Shell) (string, error) {
	switch {
	case v.Literal != nil:
		return v.Literal.Value(), nil
	case v.DollarStmt != nil:
		return v.DollarStmt.Eval(sh)
	case v.EnvVar != nil:
		return sh.GetVar(v.EnvVar.Value()[1:]), nil
	case v.String != nil:
		tokStream, err := tokeniseString(v.String.Value())
		if err != nil {
			return "", err
		}
		var str String
		parseErr := grammar.Parse(&str, tokStream)
		if parseErr != nil {
			return "", err
		}
		return str.Eval(sh)
	case v.Quote != nil:
		return strings.Trim(v.Quote.Value(), "'"), nil
	default:
		panic("bug!")
	}
}

type DollarStmt struct {
	grammar.Seq
	Open  Token `tok:"dollarbkt"`
	Stmt  Stmt
	Close Token `tok:"op,)"`
}

func (s *DollarStmt) Eval(sh *Shell) (string, error) {
	cmd, err := s.Stmt.Cmd.GetCommand(sh, os.Stdin)
	if err != nil {
		return "", err
	}
	b, err := cmd.Output()
	if err != nil {
		return "", err
	}
	b = bytes.TrimSuffix(b, []byte("\n"))
	return string(b), err
}

// Strings
var tokeniseString = grammar.SimpleTokeniser([]grammar.TokenDef{
	{
		Name: "quote",
		Ptn:  `"`,
	},
	{
		Name: "escaped",
		Ptn:  `\.`,
	},
	{
		Name: "envvar",
		Ptn:  `\$[a-zA-Z_][a-zA-Z0-9_-]*`,
	},
	{
		Name: "dollarbkt",
		Ptn:  `\$\(`,
	},
	{
		Name: "op",
		Ptn:  `[)]`,
	},
	{
		Name: "lit",
		Ptn:  `[^\\$"]+`,
	},
})

type String struct {
	grammar.Seq
	Open   *Token `tok:"quote"`
	Chunks []StringChunk
	Close  *Token `tok:"quote"`
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
