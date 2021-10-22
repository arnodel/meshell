package main

import "github.com/arnodel/grammar"

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
		Ptn:  `\d?>&?|\d?>>|&>>?|<`,
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
		Name: "literal",
		Ptn:  `[^\s();&\$|}]+`,
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
	},
})
