// Copyright (c) Bartłomiej Płotka @bwplotka
// Licensed under the Apache License 2.0.

// Taken from Thanos project.
//
// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package flagarize

import (
	"fmt"
	"io/ioutil"
	"unsafe"

	"github.com/pkg/errors"
)

// PathOrContent is a flag type that defines two flags to fetch bytes. Either from file (*-file flag) or content (* flag).
type PathOrContent struct {
	flagName string

	required bool

	path    *string
	content *string
}

func NewPathOrContent(flagName string, required bool, path, content *string) *PathOrContent {
	return &PathOrContent{
		flagName: flagName,
		required: required,
		path:     path,
		content:  content,
	}
}

// Flagarize registers PathOrContent flag.
func (p *PathOrContent) Flagarize(r FlagRegisterer, tag *Tag, _ unsafe.Pointer) error {
	if tag == nil {
		return nil
	}

	fileFlagName := fmt.Sprintf("%s-file", tag.Name)
	contentFlagName := tag.Name

	fileHelp := fmt.Sprintf("Path to %s", tag.Help)
	p.path = r.Flag(fileFlagName, fileHelp).PlaceHolder("<file-path>").String()

	contentHelp := fmt.Sprintf("Alternative to '%s' flag (lower priority). Content of %s", fileFlagName, tag.Help)
	p.content = r.Flag(contentFlagName, contentHelp).PlaceHolder("<content>").String()

	if tag.Required {
		p.required = true
	}
	p.flagName = tag.Name
	return nil
}

func (p *PathOrContent) String() string {
	return fmt.Sprintf("flag: %s, required: %v, path: %s, content: %s", p.flagName, p.required, *p.path, *p.content)
}

// Content returns content of the file. Flag that specifies path has priority.
// It returns error if the content is empty and required flag is set to true.
func (p *PathOrContent) Content() ([]byte, error) {
	contentFlagName := p.flagName
	fileFlagName := fmt.Sprintf("%s-file", p.flagName)

	if len(*p.path) > 0 && len(*p.content) > 0 {
		return nil, errors.Errorf("both %s and %s flags set.", fileFlagName, contentFlagName)
	}

	var content []byte
	if len(*p.path) > 0 {
		c, err := ioutil.ReadFile(*p.path)
		if err != nil {
			return nil, errors.Wrapf(err, "loading YAML file %s for %s", *p.path, fileFlagName)
		}
		content = c
	} else {
		content = []byte(*p.content)
	}

	if len(content) == 0 && p.required {
		return nil, errors.Errorf("flag %s or %s is required for running this command and content cannot be empty.", fileFlagName, contentFlagName)
	}

	return content, nil
}
