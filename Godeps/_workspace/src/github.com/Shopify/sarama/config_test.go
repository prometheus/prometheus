package sarama

import "testing"

func TestDefaultConfigValidates(t *testing.T) {
	config := NewConfig()
	if err := config.Validate(); err != nil {
		t.Error(err)
	}
}
