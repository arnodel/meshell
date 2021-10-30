package main

import "github.com/arnodel/grammar"

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
		Name: "arg",
		Ptn:  `\$[0-9]+`,
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
		Mode: "cmd",
		Name: "lit",
		Ptn:  `(?:[^\\"\s();&\$|}]|\\.)+`,
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
		Name: "arg",
		Ptn:  `\$[0-9]+`,
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
})
