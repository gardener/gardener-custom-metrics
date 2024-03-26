// Package errutil provides utilities for error creation and handling
package errutil

import (
	"fmt"
)

// Wrap wraps an error, adding a prefix to the message.
//
// The message becomes "<prefixMessage>: <original message>" (the prefix itself does not need a colon at the end).
//
// If prefixMessage contains format specifiers, varargs supplies the respective values.
//
// Returns nil if err is nil.
func Wrap(prefixMessage string, err error, varargs ...any) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf(prefixMessage+": %w", append(varargs, err)...)
}
