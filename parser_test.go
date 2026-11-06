package main

import (
	"strings"
	"testing"

	"github.com/arnodel/grammar"
)

func parseLine(t *testing.T, src string) *Line {
	t.Helper()
	toks, err := tokeniseCommand(src)
	if err != nil {
		t.Fatalf("%q: tokenise: %s", src, err)
	}
	var line Line
	if err := grammar.Parse(&line, toks); err != nil {
		t.Fatalf("%q: parse: %s", src, err)
	}
	return &line
}

func TestParseLines(t *testing.T) {
	for _, src := range []string{
		"ls\n",
		"ls -l /tmp | grep foo | wc -l\n",
		"FOO=bar BAZ=1 cmd a b\n",
		"FOO=bar\n",
		"cmd >out 2>&1 <in\n",
		"a && b || c; d & e\n",
		"(cd /tmp; ls) | sort\n",
		"{ a; b; }\n",
		"if test -f x; then echo yes; elif test -d x; then echo dir; else echo no; fi\n",
		"while read l; do echo \"$l\"; done\n",
		"function f() { echo $1 ${2} $(date) \"a $x b\"; }\n",
		"echo 'single quoted' \"double $HOME\" plain$var\n",
		"\n",
		"",
	} {
		line := parseLine(t, src)
		if line.CmdList == nil {
			if strings.TrimSpace(src) != "" {
				t.Errorf("%q: no commands", src)
			}
			continue
		}
		if _, err := line.CmdList.GetCommand(); err != nil {
			t.Errorf("%q: GetCommand: %s", src, err)
		}
	}
}

func TestParseShapes(t *testing.T) {
	line := parseLine(t, "FOO=bar cmd a >out b\n")
	simple := line.CmdList.First.Cmd.First.FirstCmd.Simple
	if simple == nil {
		t.Fatal("not a simple command")
	}
	if len(simple.Assignments) != 1 || simple.Assignments[0].Dest.Value() != "FOO=" {
		t.Errorf("assignments: %+v", simple.Assignments)
	}
	vals, redirects := simple.sortParts()
	if len(vals) != 3 || len(redirects) != 1 {
		t.Errorf("got %d values and %d redirects", len(vals), len(redirects))
	}
	line = parseLine(t, "a | b && c || d; e\n")
	if n := len(line.CmdList.Rest); n != 1 {
		t.Errorf("got %d further list items, want 1", n)
	}
	logical := line.CmdList.First.Cmd
	if len(logical.Rest) != 2 || len(logical.First.Pipes) != 1 {
		t.Errorf("logical: %d rest, %d pipes", len(logical.Rest), len(logical.First.Pipes))
	}
}

// Incomplete input fails at EOF, which the REPL uses to ask for more.
func TestIncomplete(t *testing.T) {
	for _, src := range []string{"if true; then\n", "echo \"abc\n", "a |\n", "(a\n"} {
		toks, err := tokeniseCommand(src)
		if err != nil {
			t.Fatalf("%q: %s", src, err)
		}
		var line Line
		perr := grammar.Parse(&line, toks)
		if perr == nil || perr.Token.Type() != grammar.EOF.TokType {
			t.Errorf("%q: want an error at EOF, got %v", src, perr)
		}
	}
}
