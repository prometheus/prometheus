package jwt

var signingMethods = map[string]func() SigningMethod{}

// Implement SigningMethod to add new methods for signing or verifying tokens.
type SigningMethod interface {
	Verify(signingString, signature string, key interface{}) error // Returns nil if signature is valid
	Sign(signingString string, key interface{}) (string, error)    // Returns encoded signature or error
	Alg() string                                                   // returns the alg identifier for this method (example: 'HS256')
}

// Register the "alg" name and a factory function for signing method.
// This is typically done during init() in the method's implementation
func RegisterSigningMethod(alg string, f func() SigningMethod) {
	signingMethods[alg] = f
}

// Get a signing method from an "alg" string
func GetSigningMethod(alg string) (method SigningMethod) {
	if methodF, ok := signingMethods[alg]; ok {
		method = methodF()
	}
	return
}
