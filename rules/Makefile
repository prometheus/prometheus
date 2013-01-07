all: parser.y.go lexer.l.go

parser.y.go: parser.y
	go tool yacc -o parser.y.go -v "" parser.y

lexer.l.go: parser.y.go lexer.l
	golex lexer.l

test: all
	go test -i github.com/matttproud/prometheus/rules
	go test github.com/matttproud/prometheus/rules

clean:
	rm lexer.l.go parser.y.go
