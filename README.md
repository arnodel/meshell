# Meshell

How hard is it to write your own shell?  I'm trying to find out!

## Installing

Not sure you want to do that :) Anyway:

`go install github.com/arnodel/meshell`

## Features
- [x] simple commands (`ls -a`)
- [x] pipelines (`ls | grep foo`)
- [x] and, or liests (`touch foo || echo ouch`)
- [x] command lists (`sleep 10; echo "Wake up!"`)
- [x] redirects to files (`ls >my-files`, `echo onions >>shopping.txt`, `go build . 2> build_errors`)
- [x] redirect stdin (`cat <foo >bar`)
- [x] redirect to fd (`./myscript.sh 2>&1 >script_output.txt`)
- [x] command groups (`{echo "my files"; ls}`)
- [ ] subshells (`(echo "my files"; ls)`)
- [x] env variable substitutions (`echo $PATH`)
- [ ] general parameter expansion (`echo ${PATH}`) - that's a rabbit hole
- [x] command substitution (`ls $(go env GOROOT)`)
- [x] shell variables (`a=hello; echo "$a, $a!"`)
- add more to the list
