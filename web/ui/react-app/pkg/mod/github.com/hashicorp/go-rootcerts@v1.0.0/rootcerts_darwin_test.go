package rootcerts

import "testing"

func TestSystemCAsOnDarwin(t *testing.T) {
	_, err := LoadSystemCAs()
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
}

func TestCertKeychains(t *testing.T) {
	keychains := certKeychains()
	if len(keychains) != 3 {
		t.Fatalf("Expected 3 keychains, got %#v", keychains)
	}
}
