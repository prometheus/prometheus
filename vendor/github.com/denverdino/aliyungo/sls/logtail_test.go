package sls

import (
	"testing"
)

func TestLogtailConfigs(t *testing.T) {
	p := DefaultProject(t)
	_, err := p.ListConfig(0, 100)
	if err != nil {
		t.Fatalf("error list logtail configs: %v", err)
	}
}

func TestCreateLogtailConfig(t *testing.T) {
	p := DefaultProject(t)
	name := "logtail-test"
	logtailConfig := &LogtailConfig{
		Name:      name,
		InputType: "file",
		InputDetail: LogtailInput{
			LogType:       "common_reg_log",
			LogPath:       "/abc",
			FilePattern:   "*.log",
			LocalStorage:  false,
			TimeFormat:    "",
			LogBeginRegex: ".*",
			Regex:         "(.*)",
			Key:           []string{"content"},
			FilterKey:     []string{"content"},
			FilterRegex:   []string{".*"},
			TopicFormat:   "none",
		},
		OutputType: "LogService",
		Sample:     "sample",
		OutputDetail: LogtailOutput{
			LogstoreName: TestLogstoreName,
		},
	}

	_, err := p.GetConfig(logtailConfig.Name)
	if err != nil {
		if e, ok := err.(*Error); ok && e.Code == "ConfigNotExist" {
			if err := p.CreateConfig(logtailConfig); err != nil {
				t.Fatalf("error create logtail config: %v", err)
			}
		} else {
			t.Fatalf("Get config error: %v", err)
		}
	}

	if err := p.DeleteConfig(name); err != nil {
		t.Fatalf("error delete logtail config: %v", err)
	}
}
