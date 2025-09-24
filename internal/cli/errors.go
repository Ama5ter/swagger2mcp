package cli

import "errors"

var ErrUsage = errors.New("cli usage error")

type usageError struct {
	msg string
}

func newUsageError(msg string) error {
	return usageError{msg: msg}
}

func (e usageError) Error() string {
	return e.msg
}

func (e usageError) Is(target error) bool {
	return target == ErrUsage
}
