package kafka

import "errors"

// permanentError marks a payload or contract failure that retrying cannot
// repair. Consumers may quarantine only these errors. Transport, database and
// other unclassified errors remain retryable and must not cross the commit
// barrier merely because a DLQ is configured.
type permanentError struct {
	err error
}

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks err as safe to quarantine after the DLQ write is durable.
func Permanent(err error) error {
	if err == nil || IsPermanent(err) {
		return err
	}
	return permanentError{err: err}
}

// IsPermanent reports whether retrying the same payload can never succeed.
func IsPermanent(err error) bool {
	var target permanentError
	return errors.As(err, &target)
}
