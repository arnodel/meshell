package main

import (
	"strings"

	"github.com/arnodel/grammar"
)

type Line struct {
	grammar.Seq
	Pfx     *Token `tok:"nl"`
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
	Op  Token `tok:"term|nl|closebrace*|closebkt*|EOF*"`
}

func (c *CmdListItem) GetCommand() (Command, error) {
	cmd, err := c.Cmd.GetCommand()
	if err != nil {
		return nil, err
	}
	if c.Op.Type() == "EOF" {
		return cmd, nil
	}
	switch c.Op.Value()[0] {
	case '&':
		return &BackgroundCommand{Cmd: cmd}, nil
	case '\n', ';', '}', ')':
		return cmd, nil
	default:
		panic("bug!")
	}
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
	Simple       *SimpleCmd
	Group        *CmdGroup
	Subshell     *Subshell
	IfStmt       *IfStmt
	WhileStmt    *WhileStmt
	FunctionStmt *FunctionStmt
}

func (i *PipelineItem) GetCommand() (Command, error) {
	switch {
	case i.Simple != nil:
		return i.Simple.GetCommand()
	case i.Group != nil:
		return i.Group.GetCommand()
	case i.Subshell != nil:
		return i.Subshell.GetCommand()
	case i.IfStmt != nil:
		return i.IfStmt.GetCommand()
	case i.WhileStmt != nil:
		return i.WhileStmt.GetCommand()
	case i.FunctionStmt != nil:
		return i.FunctionStmt.GetCommand()
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
	grammar.Seq `drop:"spc"`
	Assignments []Assignment `sep:"spc"`
	Parts       []CmdPart    `sep:"spc"`
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
	grammar.Seq `drop:"spc"`
	Op          Token `tok:"redirect"`
	File        Value
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

type IfStmt struct {
	grammar.Seq `drop:"spc|nl"`
	If          Token `tok:"kw,if"`
	Condition   CmdList
	Then        Token `tok:"kw,then"`
	ThenBody    CmdList
	ElifClauses []ElifClause
	ElseClause  *ElseClause
	Fi          Token `tok:"kw,fi"`
}

func (s *IfStmt) GetCommand() (Command, error) {
	cond, err := s.Condition.GetCommand()
	if err != nil {
		return nil, err
	}
	thenCmd, err := s.ThenBody.GetCommand()
	if err != nil {
		return nil, err
	}
	ifCmd := &IfCommand{
		Condition: cond,
		Then:      thenCmd,
	}
	cmd := ifCmd
	for _, c := range s.ElifClauses {
		cond, err = c.Condition.GetCommand()
		if err != nil {
			return nil, err
		}
		thenCmd, err := c.ThenBody.GetCommand()
		if err != nil {
			return nil, err
		}
		newCmd := &IfCommand{
			Condition: cond,
			Then:      thenCmd,
		}
		cmd.Else = newCmd
		newCmd = cmd
	}
	if s.ElseClause != nil {
		elseCmd, err := s.ElseClause.Body.GetCommand()
		if err != nil {
			return nil, err
		}
		cmd.Else = elseCmd
	}
	return ifCmd, nil
}

type ElifClause struct {
	grammar.Seq `drop:"spc|nl"`
	Elif        Token `tok:"kw,elif"`
	Condition   CmdList
	Then        Token `tok:"kw,then"`
	ThenBody    CmdList
}

type ElseClause struct {
	grammar.Seq `drop:"spc|nl"`
	Else        Token `tok:"kw,else"`
	Body        CmdList
}

type WhileStmt struct {
	grammar.Seq `drop:"spc|nl"`
	While       Token `tok:"kw,while"`
	Condition   CmdList
	Do          Token `tok:"kw,do"`
	Body        CmdList
	Done        Token `tok:"kw,done"`
}

func (s *WhileStmt) GetCommand() (Command, error) {
	cond, err := s.Condition.GetCommand()
	if err != nil {
		return nil, err
	}
	body, err := s.Body.GetCommand()
	if err != nil {
		return nil, err
	}
	return &WhileCommand{
		Condition: cond,
		Body:      body,
	}, nil
}

type FunctionStmt struct {
	grammar.Seq `drop:"spc"`
	Function    Token `tok:"kw,function"`
	Name        Value
	OpenBkt     Token `tok:"openbkt"`
	CloseBkt    Token `tok:"closebkt"`

	Body PipelineItem
}

func (s *FunctionStmt) GetCommand() (Command, error) {
	name, err := s.Name.Eval()
	if err != nil {
		return nil, err
	}
	body, err := s.Body.GetCommand()
	if err != nil {
		return nil, err
	}
	return &FunctionDefCommand{
		Name: name,
		Body: body,
	}, nil
}

type Pipeline struct {
	grammar.Seq `drop:"spc"`
	Start       *grammar.Empty
	FirstCmd    PipelineItem
	Pipes       []PipedCmd
	End         *grammar.Empty
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
	String      *String
	Quote       *Token `tok:"litstr"`
	StringChunk *StringChunk
}

func (v *SingleValue) Eval() (ValueDef, error) {
	switch {
	case v.String != nil:
		return v.String.Eval()
	case v.Quote != nil:
		return LiteralValueDef{
			Val:    strings.Trim(v.Quote.Value(), "'"),
			Expand: false,
		}, nil
	case v.StringChunk != nil:
		return v.StringChunk.Eval(false)
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

type DollarBrace struct {
	grammar.Seq
	Open      Token `tok:"dollarbrace"`
	ParamName Token `tok:"name|argnum|special"`
	Close     Token `tok:"closebrace"`
}

func (s *DollarBrace) Eval() (ValueDef, error) {
	return ParamValueDef(s.ParamName.Value())
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
		parts[i], err = chunk.Eval(true)
		if err != nil {
			return nil, err
		}
	}
	return CompositeValueDef{Parts: parts}, nil
}

type StringChunk struct {
	grammar.OneOf
	Lit         *Token `tok:"lit"`
	DollarStmt  *DollarStmt
	DollarBrace *DollarBrace
	Param       *Token `tok:"envvar|specialvar"`
}

func (c *StringChunk) Eval(inString bool) (ValueDef, error) {
	switch {
	case c.Lit != nil:
		return LiteralValueDef{Val: UnescapeLiteral(c.Lit.Value(), inString), Expand: true}, nil
	case c.DollarStmt != nil:
		return c.DollarStmt.Eval()
	case c.DollarBrace != nil:
		return c.DollarBrace.Eval()
	case c.Param != nil:
		return ParamValueDef(c.Param.Value()[1:])
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
