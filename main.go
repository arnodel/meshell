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

type Shell struct {
	cwd    string
	vars   map[string]string
	exitCh chan int
}

func NewShell(cwd string) *Shell {
	return &Shell{
		cwd:    cwd,
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
	return c.Start(s)
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
		Ptn: `[ \t]+`,
	},
	{
		Ptn: `\\\n`,
	},
	{
		Name: "logical",
		Ptn:  `&&|\|\|`,
	},
	{
		Ptn:  `[;&\n]\s*`,
		Name: "term",
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
		Name: "pipe",
		Ptn:  `\|\s*`,
	},
	{
		Name: "closebkt",
		Ptn:  `\)`,
	},
	{
		Name: "redirect",
		Ptn:  `>>|>|<<|<`,
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
		Ptn:  `[^\s();&\$|]+`,
	},
})

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
	Op  Token `tok:"term"`
}

func (c *CmdListItem) GetCommand(sh *Shell, stdin io.ReadCloser) (Command, error) {
	cmd, err := c.Cmd.GetCommand(sh, stdin)
	if err != nil {
		return nil, err
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
		switch next.Op.Value() {
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
	Parts       []CmdPart `size:"1-"`
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
	String     *Token `tok:"string"`
	Quote      *Token `tok:"quote"`
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
		return []string{s}, err
	case v.EnvVar != nil:
		return []string{sh.GetVar(v.EnvVar.Value()[1:])}, nil
	case v.String != nil:
		tokStream, err := tokeniseString(v.String.Value())
		if err != nil {
			return nil, err
		}
		var str String
		parseErr := grammar.Parse(&str, tokStream)
		if parseErr != nil {
			return nil, err
		}
		s, err := str.Eval(sh)
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
