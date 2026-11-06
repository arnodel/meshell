package main

import (
	"regexp"

	"github.com/arnodel/grammar"
)

type Token = grammar.SimpleToken

var tokeniseCommand = grammar.SimpleTokeniser([]grammar.TokenDef{
	//
	// Command
	//
	{
		Mode: "cmd",
		Name: "spc",
		Ptn:  `[ \t]+`,
	},
	{
		Mode: "cmd",
		Ptn:  `\\\n[ \t]*`,
	},
	{
		Mode: "cmd",
		Name: "logical",
		Ptn:  `(?:&&|\|\|)\s*`,
	},
	{
		Mode: "cmd",
		Ptn:  `\n\s*`,
		Name: "nl",
	},
	{
		Mode: "cmd",
		Ptn:  `[;&]\s*`,
		Name: "term",
	},

	{
		Mode: "cmd",
		Name: "envvar",
		Ptn:  `\$[a-zA-Z_][a-zA-Z0-9_-]*`,
	},
	{
		Mode: "cmd",
		Name: "specialvar",
		Ptn:  `\$(?:[0-9]+|[?#@$])`,
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
		Mode:     "cmd",
		Name:     "dollarbrace",
		Ptn:      `\$\{`,
		PushMode: "param",
	},
	{
		Mode: "cmd",
		Name: "pipe",
		Ptn:  `\|\s*`,
	},
	{
		Mode:     "cmd",
		Name:     "openbkt",
		Ptn:      `\(\s*`,
		PushMode: "cmd",
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
		Ptn:  `\d?>>|\d?>&?|&>>?|<`,
	},
	{
		Mode: "cmd",
		Name: "openbrace",
		Ptn:  `{\s*`,
	},
	{
		Mode: "cmd",
		Name: "closebrace",
		Ptn:  `\}`,
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
		Name: "kw",
		Ptn:  `(?:if|then|elif|else|fi|while|do|done|function)\b`,
	},
	{
		// The tokeniser takes the longest match, so a literal must not be
		// able to swallow a redirection ("2>file") or an assignment
		// ("FOO=bar"), which are shorter.
		Mode: "cmd",
		Name: "lit",
		Ptn:  `(?:[^\\"\s();&\$|}<>]|\\.)+`,
		Scan: scanLit,
	},
	//
	// String
	//
	{
		Mode:    "str",
		Name:    "endquote",
		Ptn:     `"`,
		PopMode: true,
	},
	{
		Mode: "str",
		Name: "envvar",
		Ptn:  `\$[a-zA-Z_][a-zA-Z0-9_-]*`,
	},
	{
		Mode: "str",
		Name: "specialvar",
		Ptn:  `\$(?:[0-9]+|[?#@$])`,
	},
	{
		Mode:     "str",
		Name:     "dollarbkt",
		Ptn:      `\$\(\s*`,
		PushMode: "cmd",
	},
	{
		Mode:     "str",
		Name:     "dollarbrace",
		Ptn:      `\$\{`,
		PushMode: "param",
	},
	{
		Mode: "str",
		Name: "lit",
		Ptn:  `(?:[^\\$"]|\\.)+`,
	},
	//
	// Parameter
	//
	{
		Mode:    "param",
		Name:    "closebrace",
		Ptn:     `}`,
		PopMode: true,
	},
	{
		Mode: "param",
		Name: "name",
		Ptn:  `[a-zA-Z_][a-zA-Z0-9_-]*`,
	},
	{
		Mode: "param",
		Name: "argnum",
		Ptn:  `[0-9]+`,
	},
	{
		Mode: "param",
		Name: "special",
		Ptn:  `[?#@$]`,
	},
})

var (
	litPtn    = regexp.MustCompile(`^(?:[^\\"\s();&\$|}<>]|\\.)+`)
	assignPtn = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*=`)
)

// scanLit is the length of a "lit" token at the start of s, or 0 if s starts
// with an assignment.
func scanLit(s string) int {
	if assignPtn.MatchString(s) {
		return 0
	}
	return len(litPtn.FindString(s))
}
