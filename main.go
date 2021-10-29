package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/arnodel/grammar"
	"github.com/peterh/liner"
)

func main() {
	linr := liner.NewLiner()
	defer linr.Close()
	linr.SetCtrlCAborts(true)

	shell := NewShell()
outerLoop:
	for {
		cwd, _ := shell.GetCwd()
		line, err := linr.Prompt(fmt.Sprintf("%s$ ", cwd))
		if err == io.EOF {
			fmt.Fprintln(os.Stdout, "\nBye!")
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
			parseErr := grammar.Parse(&parsedLine, tokenStream) //, grammar.WithDefaultLogger)
			// tokenStream.Dump(os.Stdout)
			if parseErr == nil {
				linr.AppendHistory(strings.TrimSpace(line))
				if parsedLine.CmdList == nil {
					continue outerLoop
				}
				cmdDef, err := parsedLine.CmdList.GetCommand()
				var job RunningJob
				if err == nil {
					job, err = cmdDef.StartJob(shell, StdStreams{
						In:  os.Stdin,
						Out: os.Stdout,
						Err: os.Stderr,
					})
				}
				if err == nil {
					res := job.Wait()
					if !res.Success() {
						err = res
					}
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
