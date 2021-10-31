package main

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func UnescapeLiteral(s string, inString bool) string {
	if inString {
		return stringEscapeSeqs.ReplaceAllStringFunc(s, replaceStringEscapeSeq)
	}
	return literalEscapeSeqs.ReplaceAllStringFunc(s, replaceLiteralEscapeSeq)
}

// This function replaces escape sequences with the values they escape.
// Lifted from Golua, probably needs adapting better to a shell context.
func replaceStringEscapeSeq(e string) string {
	switch e[1] {
	case 'a':
		return "\a"
	case 'b':
		return "\b"
	case 't':
		return "\t"
	case 'n':
		return "\n"
	case 'v':
		return "\v"
	case 'f':
		return "\f"
	case 'r':
		return "\r"
	case 'x', 'X':
		b, err := strconv.ParseInt(e[2:], 16, 64)
		if err != nil {
			panic(err)
		}
		return string(rune(b))
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		b, err := strconv.ParseInt(string(e[1:]), 10, 64)
		if err != nil {
			panic(err)
		}
		if b >= 256 {
			panic(errors.New("decimal escape sequence out of range"))
		}
		return string(rune(b))
	case 'u', 'U':
		i, err := strconv.ParseInt(string(e[3:len(e)-1]), 16, 32)
		if err != nil {
			panic(err)
		}
		if i >= 0x110000 {
			panic(errors.New("unicode escape sequence out of range"))
		}
		return string(rune(i))
	default:
		return e
	}
}

// This regex matches all the escape sequences that can be found in a Lua string.
var stringEscapeSeqs = regexp.MustCompile(`(?s)\\\d{1,3}|\\[xX][0-9a-fA-F]{2}|\\[abtnvfr\\]|\\[uU]{[0-9a-fA-F]+}|\\.`)

func replaceLiteralEscapeSeq(e string) string {
	switch e[1] {
	case '\n':
		return ""
	default:
		return e[1:]
	}
}

var literalEscapeSeqs = regexp.MustCompile(`\\.`)

//
// The following is lifted and slightly adapted from the go package exec
// (lp_unix.go).  I can't reuse it as is because of the use of os.Getenv("PATH")
// in the original code.
//

func findExecutable(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if m := d.Mode(); !m.IsDir() && m&0111 != 0 {
		return nil
	}
	return fs.ErrPermission
}

// LookPath searches for an executable named file in the
// directories named by the path.
// If file contains a slash, it is tried directly and path is not consulted.
// The result may be an absolute path or a path relative to the current directory.
func LookPath(path, wd, file string) (string, error) {
	if strings.HasPrefix(file, "/") {
		err := findExecutable(file)
		if err == nil {
			return file, nil
		}
		return "", &exec.Error{Name: file, Err: err}
	}
	if strings.Contains(file, "/") {
		path := filepath.Join(wd, file)
		err := findExecutable(path)
		if err == nil {
			return path, nil
		}
		return "", &exec.Error{Name: file, Err: err}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			// Unix shell semantics: path element "" means "."
			dir = wd
		}
		path := filepath.Join(dir, file)
		if err := findExecutable(path); err == nil {
			return path, nil
		}
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

//
// End of lifting
//

var ErrNotADirectory = errors.New("file is not a directory")

func findDirectory(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if m := d.Mode(); m.IsDir() {
		return nil
	}
	return ErrNotADirectory
}

func LookDir(wd, file string) (string, error) {
	if strings.HasPrefix(file, "/") {
		err := findDirectory(file)
		if err == nil {
			return file, nil
		}
		return "", &exec.Error{Name: file, Err: err}
	}
	path := filepath.Join(wd, file)
	err := findDirectory(path)
	if err == nil {
		return path, nil
	}
	return "", &exec.Error{Name: file, Err: err}
}
