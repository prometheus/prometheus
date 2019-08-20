package util

import (
	"testing"
)

func TestCreateRandomString(t *testing.T) {
	for i := 0; i < 10; i++ {
		s := CreateRandomString()
		t.Logf("Generated Random String: %s", s)
	}
}

func TestGenerateRandomECSPassword(t *testing.T) {
	for i := 0; i < 10; i++ {
		s := GenerateRandomECSPassword()

		if len(s) < 8 || len(s) > 30 {
			t.Errorf("Generated ECS password [%v]: bad len", s)
		}

		hasDigit := false
		hasLower := false
		hasUpper := false

		for j := range s {

			switch {
			case '0' <= s[j] && s[j] <= '9':
				hasDigit = true
			case 'a' <= s[j] && s[j] <= 'z':
				hasLower = true
			case 'A' <= s[j] && s[j] <= 'Z':
				hasUpper = true
			}
		}

		if !hasDigit {
			t.Errorf("Generated ECS password [%v]: no digit", s)
		}

		if !hasLower {
			t.Errorf("Generated ECS password [%v]: no lower letter ", s)
		}

		if !hasUpper {
			t.Errorf("Generated ECS password [%v]: no upper letter", s)
		}
	}
}
