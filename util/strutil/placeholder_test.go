package strutil

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestIp(t *testing.T) {
	str := "asdf ${buildin_ip}"
	replaced := ReplacePlaceholders(str)
	fmt.Print(replaced)
}
func TestOs(t *testing.T) {
	os.Setenv("aaa", "hi")
	str := "the replaced str: ${os_aaa}"
	replaced := ReplacePlaceholders(str)
	assert.Equal(t, "the replaced str: hi", replaced)
	fmt.Print(replaced)
}
