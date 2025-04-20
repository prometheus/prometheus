package rules

import (
	"os"

	"gopkg.in/yaml.v3"
)

func ParseFile(fn string) (interface{}, error) {
	content, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	var rules interface{}
	err = yaml.Unmarshal(content, &rules)
	if err != nil {
		return nil, err
	}

	return rules, nil
}
