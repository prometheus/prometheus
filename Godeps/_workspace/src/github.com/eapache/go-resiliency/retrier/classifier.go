package retrier

// Action is the type returned by a Classifier to indicate how the Retrier should proceed.
type Action int

const (
	Succeed Action = iota // Succeed indicates the Retrier should treat this value as a success.
	Fail                  // Fail indicates the Retrier should treat this value as a hard failure and not retry.
	Retry                 // Retry indicates the Retrier should treat this value as a soft failure and retry.
)

// Classifier is the interface implemented by anything that can classify Errors for a Retrier.
type Classifier interface {
	Classify(error) Action
}

// DefaultClassifier classifies errors in the simplest way possible. If
// the error is nil, it returns Succeed, otherwise it returns Retry.
type DefaultClassifier struct{}

// Classify implements the Classifier interface.
func (c DefaultClassifier) Classify(err error) Action {
	if err == nil {
		return Succeed
	}

	return Retry
}

// WhitelistClassifier classifies errors based on a whitelist. If the error is nil, it
// returns Succeed; if the error is in the whitelist, it returns Retry; otherwise, it returns Fail.
type WhitelistClassifier []error

// Classify implements the Classifier interface.
func (list WhitelistClassifier) Classify(err error) Action {
	if err == nil {
		return Succeed
	}

	for _, pass := range list {
		if err == pass {
			return Retry
		}
	}

	return Fail
}

// BlacklistClassifier classifies errors based on a blacklist. If the error is nil, it
// returns Succeed; if the error is in the blacklist, it returns Fail; otherwise, it returns Retry.
type BlacklistClassifier []error

// Classify implements the Classifier interface.
func (list BlacklistClassifier) Classify(err error) Action {
	if err == nil {
		return Succeed
	}

	for _, pass := range list {
		if err == pass {
			return Fail
		}
	}

	return Retry
}
