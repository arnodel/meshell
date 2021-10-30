package main

import (
	"errors"
	"regexp"
	"strconv"
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
