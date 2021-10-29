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

func (c *CmdList) GetCommand() (Command, error) {
	cmdSeq, err := c.First.GetCommand()
	if err != nil {
		return nil, err
	}
	for _, item := range c.Rest {
		cmd, err := item.GetCommand()
		if err != nil {
			return nil, err
		}
		cmdSeq = &CommandSequence{
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
	Op  Token `tok:"term|closebrace*|closebkt*"`
}

func (c *CmdListItem) GetCommand() (Command, error) {
	cmd, err := c.Cmd.GetCommand()
	if err != nil {
		return nil, err
	}
	switch c.Op.Value()[0] {
	case '&':
		cmd = &BackgroundCommand{Cmd: cmd}
	case '\n', ';', '}', ')':
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

func (c *CmdLogical) GetCommand() (Command, error) {
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
		cmdSeq = &CommandSequence{
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

type PipelineItem struct {
	grammar.OneOf
	Simple   *SimpleCmd
	Group    *CmdGroup
	Subshell *Subshell
}

func (i *PipelineItem) GetCommand() (Command, error) {
	switch {
	case i.Simple != nil:
		return i.Simple.GetCommand()
	case i.Group != nil:
		return i.Group.GetCommand()
	case i.Subshell != nil:
		return i.Subshell.GetCommand()
	default:
		panic("bug!")
	}
}

type CmdGroup struct {
	grammar.Seq
	Open  Token `tok:"openbrace"`
	Cmds  CmdList
	Close Token `tok:"closebrace"`
}

func (g *CmdGroup) GetCommand() (Command, error) {
	return g.Cmds.GetCommand()
}

type Subshell struct {
	grammar.Seq
	Open  Token `tok:"openbkt"`
	Cmds  CmdList
	Close Token `tok:"closebkt"`
}

func (s *Subshell) GetCommand() (Command, error) {
	body, err := s.Cmds.GetCommand()
	if err != nil {
		return nil, err
	}
	return &SubshellCommand{Body: body}, nil
}

type SimpleCmd struct {
	grammar.Seq
	Separator   Token `tok:"spc"`
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
	Op   Token  `tok:"redirect"`
	Spc  *Token `tok:"spc"`
	File Value
}

func (c *SimpleCmd) GetCommand() (Command, error) {
	args, redirects := c.sortParts()
	parts := make([]ValueDef, len(args))
	for i, arg := range args {
		val, err := arg.Eval()
		if err != nil {
			return nil, err
		}
		parts[i] = val
	}
	env := make([]AssignDef, len(c.Assignments))
	for i, a := range c.Assignments {
		val, err := a.Value.Eval()
		if err != nil {
			return nil, err
		}
		env[i] = AssignDef{
			Name: getAssignDest(a.Dest.Value()),
			Val:  val,
		}
	}
	var cmd Command
	if len(parts) == 0 {
		cmd = &SetVarsCommand{
			Assigns: env,
		}
	} else {
		cmd = &SimpleCommand{
			CmdName: parts[0],
			Args:    parts[1:],
			Assigns: env,
		}
	}
	for i := len(redirects) - 1; i >= 0; i-- {
		r := redirects[i]
		repl, err := r.File.Eval()
		if err != nil {
			return nil, err
		}
		op, fd, ref := splitRedirect(r.Op.Value())
		var mode int
		switch op {
		case ">":
			if fd == -1 {
				fd = 1
			}
			mode = RM_Truncate
		case ">>":
			if fd == -1 {
				fd = 1
			}
			fd = 1
			mode = RM_Append
		case "<":
			fd = 0
			mode = RM_Read
		default:
			panic("bug!")
		}
		cmd = &RedirectCommand{
			Cmd:         cmd,
			Replacement: repl,
			FD:          fd,
			Mode:        mode,
			Ref:         ref,
		}
	}
	return cmd, nil
}

type Assignment struct {
	grammar.Seq
	Dest  Token `tok:"assign"`
	Value Value
}

type Pipeline struct {
	grammar.Seq
	Separator *Token `tok:"spc"`
	Start     *grammar.Empty
	FirstCmd  PipelineItem
	Pipes     []PipedCmd
	End       *grammar.Empty
}

func (c *Pipeline) GetCommand() (Command, error) {
	cmd, err := c.FirstCmd.GetCommand()
	if err != nil {
		return nil, err
	}
	for _, pipe := range c.Pipes {
		right, err := pipe.Cmd.GetCommand()
		if err != nil {
			return nil, err
		}
		cmd = &PipelineCommand{Left: cmd, Right: right}
	}
	return cmd, nil
}

type PipedCmd struct {
	grammar.Seq
	Pipe Token `tok:"pipe"`
	Cmd  PipelineItem
}

type Value struct {
	grammar.Seq
	Components []SingleValue
}

func (v *Value) Eval() (ValueDef, error) {
	if len(v.Components) == 1 {
		return v.Components[0].Eval()
	}
	components := make([]ValueDef, len(v.Components))
	for i, c := range v.Components {
		v, err := c.Eval()
		if err != nil {
			return nil, err
		}
		components[i] = v
	}
	return CompositeValueDef{Parts: components}, nil
}

type SingleValue struct {
	grammar.OneOf
	Literal    *Token `tok:"literal"`
	String     *String
	Quote      *Token `tok:"litstr"`
	EnvVar     *Token `tok:"envvar"`
	DollarStmt *DollarStmt
}

func (v *SingleValue) Eval() (ValueDef, error) {
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

func splitRedirect(op string) (string, int, bool) {
	origFd := -1
	if op[0] >= '0' && op[0] <= '9' {
		origFd = int(op[0] - '0')
		op = op[1:]
	}
	l := len(op) - 1
	ref := op[l] == '&'
	if ref {
		op = op[:l]
	}
	return op, origFd, ref
}
