package main

import (
	"strconv"
	"strings"

	"github.com/arnodel/grammar"
)

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

func (c *CmdList) GetCommand() (CommandDef, error) {
	cmdSeq, err := c.First.GetCommand()
	if err != nil {
		return nil, err
	}
	for _, item := range c.Rest {
		cmd, err := item.GetCommand()
		if err != nil {
			return nil, err
		}
		cmdSeq = SeqCmdDef{
			Left:    cmdSeq,
			Right:   cmd,
			SeqType: UncondSeq,
		}
	}
	return cmdSeq, nil
}

type CmdListItem struct {
	grammar.Seq
	Cmd CmdLogical
	Op  *Token `tok:"term"`
}

func (c *CmdListItem) GetCommand() (CommandDef, error) {
	cmd, err := c.Cmd.GetCommand()
	if err != nil {
		return nil, err
	}
	if c.Op == nil {
		return cmd, err
	}
	switch c.Op.Value()[0] {
	case '&':
		cmd = BackgroundCmdDef{Cmd: cmd}
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

func (c *CmdLogical) GetCommand() (CommandDef, error) {
	cmdSeq, err := c.First.GetCommand()
	if err != nil {
		return nil, err
	}
	for _, next := range c.Rest {
		cmd, err := next.Cmd.GetCommand()
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
		cmdSeq = SeqCmdDef{
			Left:    cmdSeq,
			Right:   cmd,
			SeqType: op,
		}
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

func (c *SimpleCmd) GetCommand() (CommandDef, error) {
	args, redirects := c.sortParts()
	// TODO: deal with redirects
	_ = redirects
	parts := make([]ValueDef, len(args))
	for i, arg := range args {
		val, err := arg.Eval()
		if err != nil {
			return nil, err
		}
		parts[i] = val
	}
	env := make([]VarDef, len(c.Assignments))
	for i, a := range c.Assignments {
		val, err := a.Value.Eval()
		if err != nil {
			return nil, err
		}
		env[i] = VarDef{
			Name: getAssignDest(a.Dest.Value()),
			Val:  val,
		}
	}
	return &ExecCmdDef{
		Parts: parts,
		Env:   env,
	}, nil
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

type Pipeline struct {
	grammar.Seq
	FirstCmd SimpleCmd
	Pipes    []PipedCmd
}

func (c *Pipeline) GetCommand() (CommandDef, error) {
	cmd, err := c.FirstCmd.GetCommand()
	if err != nil {
		return nil, err
	}
	for _, pipe := range c.Pipes {
		right, err := pipe.Cmd.GetCommand()
		if err != nil {
			return nil, err
		}
		cmd = &PipelineCmdDef{Left: cmd, Right: right}
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

func (v *Value) Eval() (ValueDef, error) {
	switch {
	case v.Literal != nil:
		return LiteralValueDef{
			Val:    v.Literal.Value(),
			Expand: true,
		}, nil
	case v.DollarStmt != nil:
		return v.DollarStmt.Eval()
	case v.EnvVar != nil:
		return VarValueDef{Name: v.EnvVar.Value()[1:]}, nil
	case v.String != nil:
		return v.String.Eval()
	case v.Quote != nil:
		return LiteralValueDef{
			Val:    strings.Trim(v.Quote.Value(), "'"),
			Expand: false,
		}, nil
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

func (s *DollarStmt) Eval() (ValueDef, error) {
	cmd, err := s.Cmds.GetCommand()
	if err != nil {
		return nil, err
	}
	return CommandValueDef{Cmd: cmd}, nil
}

type String struct {
	grammar.Seq
	Open   Token `tok:"startquote"`
	Chunks []StringChunk
	Close  Token `tok:"endquote"`
}

func (s *String) Eval() (ValueDef, error) {
	parts := make([]ValueDef, len(s.Chunks))
	var err error
	for i, chunk := range s.Chunks {
		parts[i], err = chunk.Eval()
		if err != nil {
			return nil, err
		}
	}
	return CompositeValueDef{Parts: parts}, nil
}

type StringChunk struct {
	grammar.OneOf
	Lit        *Token `tok:"lit"`
	DollarStmt *DollarStmt
	EnvVar     *Token `tok:"envvar"`
	Escaped    *Token `tok:"escaped"`
}

func (c *StringChunk) Eval() (ValueDef, error) {
	switch {
	case c.Lit != nil:
		return LiteralValueDef{Val: c.Lit.Value()}, nil
	case c.DollarStmt != nil:
		return c.DollarStmt.Eval()
	case c.EnvVar != nil:
		return VarValueDef{Name: c.EnvVar.Value()[1:]}, nil
	case c.Escaped != nil:
		r, _, _, err := strconv.UnquoteChar(c.Escaped.Value(), '"')
		if err != nil {
			return nil, err
		}
		return LiteralValueDef{Val: string(r)}, nil
	default:
		panic("bug!")
	}
}

func getAssignDest(s string) string {
	return s[:len(s)-1]
}
