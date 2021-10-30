# Meshell

How hard is it to write your own shell?  I'm trying to find out!

## Installing

Not sure you want to do that :) Anyway:

`go install github.com/arnodel/meshell`

## Features
- [x] `cd` builtin
- [x] `exit` builtin
- [x] `shift` builtin
- [x] simple commands (`ls -a`)
- [x] pipelines (`ls | grep foo`)
- [x] and, or lists (`touch foo || echo ouch`)
- [x] command lists (`sleep 10; echo "Wake up!"`)
- [x] redirects to files (`ls >my-files`, `echo onions >>shopping.txt`, `go build . 2> build_errors`)
- [x] redirect stdin (`cat <foo >bar`)
- [x] redirect to fd (`./myscript.sh 2>&1 >script_output.txt`)
- [x] command groups (`{echo "my files"; ls}`)
- [x] subshells (`(a=12; echo $a)`)
- [x] env variable substitutions (`echo $PATH`)
- [x] simple parameter substitution (`echo ${var}`)
- [ ] general parameter expansion (`echo ${PATH:stuff}`) - that's a rabbit hole
- [x] command substitution (`ls $(go env GOROOT)`)
- [x] shell variables (`a=hello; echo "$a, $a!"`)
- [x] functions with `return` (`function foo() {echo $2; return; echo $1}; foo hello there `)
- [x] if then else
- [x] while loops
- [ ] for loops
- [ ] export (`export a=10`)
- [x] arguments (`echo $1 ${2}`)
- add more to the list
