all: parser.y.go lexer.l.go

parser.y.go: parser.y
	go tool yacc -o parser.y.go -v "" parser.y

lexer.l.go: parser.y.go lexer.l
	golex lexer.l

clean:
	rm lexer.l.go parser.y.go
