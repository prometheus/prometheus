package endpoints

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getEndpointConfigData(t *testing.T) {
	r := getEndpointConfigData()
	d, ok := r.(map[string]interface{})
	assert.True(t, ok)
	products := d["products"]
	p, ok := products.([]interface{})
	assert.True(t, ok)
	assert.Equal(t, 127, len(p))
}
