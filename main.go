package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/arnodel/grammar"
	"github.com/peterh/liner"
)

func main() {
	os.Exit(run())
}

func run() int {
	debug := os.Getenv("DEBUG") != ""
	var parseOpts []grammar.ParseOption
	if debug {
		parseOpts = append(parseOpts, grammar.WithDefaultLogger)
	}
	flag.Parse()
	var (
		filename string
		script   []byte
		err      error
		args     []string
	)
	switch flag.NArg() {
	case 0:
		filename = "<stdin>"
		if isaTTY(os.Stdin) {
			return repl(debug, parseOpts)
		}
		args = []string{"meshell"}
		script, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			fatal("Error reading <stdin>: %s", err)
		}
	default:
		filename = flag.Arg(0)
		script, err = ioutil.ReadFile(filename)
		if err != nil {
			return fatal("Error reading '%s': %s", filename, err)
		}
		args = flag.Args()
	}
	tokenStream, err := tokeniseCommand(string(script))
	if err != nil {
		return fatal("error parsing %s: %s", filename, err)
	}
	var line Line
	parseErr := grammar.Parse(&line, tokenStream, parseOpts...)
	if debug {
		tokenStream.Dump(os.Stdout)
	}
	if parseErr != nil {
		return fatal("error parsing %s: %s", filename, parseErr)
	}
	if line.CmdList == nil {
		return 0
	}
	cmdDef, err := line.CmdList.GetCommand()
	if err != nil {
		return fatal("error interpreting %s: %s", filename, err)
	}
	cwd, _ := os.Getwd()
	shell := NewShell(args[0], args[1:], cwd)
	job, err := cmdDef.StartJob(shell, StdStreams{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	})
	if err == nil {
		res := job.Wait()
		if !res.Success() {
			err = res
		}
	}
	if err != nil {
		return fatal("error running %s: %s", filename, err)
	}
	return 0
}

func isaTTY(f *os.File) bool {
	fi, _ := f.Stat()
	return fi.Mode()&os.ModeCharDevice != 0
}

func fatal(tpl string, args ...interface{}) int {
	fmt.Fprintf(os.Stderr, tpl, args...)
	return 1
}

func repl(debug bool, parseOpts []grammar.ParseOption) int {

	linr := liner.NewLiner()
	defer linr.Close()
	linr.SetCtrlCAborts(true)
	cwd, _ := os.Getwd()
	shell := NewShell(os.Args[0], nil, cwd)
outerLoop:
	for {
		line, err := linr.Prompt(fmt.Sprintf("%s$ ", shell.GetCwd()))
		if err == io.EOF {
			fmt.Fprintln(os.Stdout, "\nBye!")
			return 0
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
			parseErr := grammar.Parse(&parsedLine, tokenStream, parseOpts...)
			if debug {
				tokenStream.Dump(os.Stdout)
			}
			if parseErr == nil {
				linr.AppendHistory(strings.TrimSpace(line))
				if parsedLine.CmdList == nil {
					continue outerLoop
				}
				cmdDef, err := parsedLine.CmdList.GetCommand()
				if err == nil {
					err = shell.RunCommand(cmdDef, StdStreams{
						In:  os.Stdin,
						Out: os.Stdout,
						Err: os.Stderr,
					})
				}
				if err != nil {
					fmt.Println(err)
				}
				if shell.Exited() {
					linr.Close()
					os.Exit(shell.Wait())
				}
				continue outerLoop
			} else if parseErr.Token == grammar.EOF {
				more, err := linr.Prompt("> ")
				if err == io.EOF {
					return 0
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
