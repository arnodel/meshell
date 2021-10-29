package main

import (
	"errors"
	"os"
	"strconv"
)

type builtinFunc func(sh *Shell, std StdStreams, args []string) (RunningJob, error)

var builtins map[string]builtinFunc

func init() {
	builtins = map[string]builtinFunc{
		"cd":   builtinCd,
		"exit": builtinExit,
	}
}

func builtinCd(sh *Shell, std StdStreams, args []string) (RunningJob, error) {
	var (
		dir = ""
		err error
	)
	switch len(args) {
	case 0:
		dir, err = os.UserHomeDir()
	case 1:
		dir = args[0]
	default:
		err = errors.New("cd: wrong number of arguments")
	}
	if err != nil {
		return nil, err
	}
	err = sh.SetCwd(dir)
	if err != nil {
		return nil, err
	}
	return &ImmediateRunningJob{name: "cd"}, nil
}

func builtinExit(sh *Shell, std StdStreams, args []string) (RunningJob, error) {
	var (
		code int64
		err  error
	)
	switch len(args) {
	case 0:
		// default exit code
	case 1:
		codeStr := args[0]
		code, err = strconv.ParseInt(codeStr, 10, 64)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("exit: wrong number of arguments")
	}
	sh.Exit(int(code))
	return &ImmediateRunningJob{name: "exit"}, nil
}
