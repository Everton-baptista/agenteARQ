package workflow

import "fmt"

// SourceValidationError identifies deterministic source-policy violations. It does
// not wrap filesystem or Git errors, which remain infrastructure failures.
type SourceValidationError struct {
	Reason string
}

func (e *SourceValidationError) Error() string { return e.Reason }

func invalidSource(format string, args ...any) error {
	return &SourceValidationError{Reason: fmt.Sprintf(format, args...)}
}
