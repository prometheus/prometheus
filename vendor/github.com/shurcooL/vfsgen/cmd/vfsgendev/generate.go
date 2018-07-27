package main

import (
	"strconv"
	"text/template"
)

type data struct {
	ImportPath      string
	PackageName     string
	BuildTags       string
	VariableName    string
	VariableComment string
}

var generateTemplate = template.Must(template.New("").Funcs(template.FuncMap{
	"quote": strconv.Quote,
}).Parse(`package main

import (
	"log"

	"github.com/shurcooL/vfsgen"

	sourcepkg {{.ImportPath | quote}}
)

func main() {
	err := vfsgen.Generate(sourcepkg.{{.VariableName}}, vfsgen.Options{
		PackageName:     {{.PackageName | quote}},
		BuildTags:       {{.BuildTags | quote}},
		VariableName:    {{.VariableName | quote}},
		VariableComment: {{.VariableComment | quote}},
	})
	if err != nil {
		log.Fatalln(err)
	}
}
`))
