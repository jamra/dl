package DLError

import (
	"fmt"
)

//DLError wraps request and response errors and gives a more descriptive name
type DLError struct {
	name       string
	innerError error
}

func (e *DLError) Error() string {
	return fmt.Sprintf("%s -> %s", e.name, e.innerError.Error())
}

//New creates a named error from the passed in string and wraps the passed in error
func New(name string, innerError error) *DLError {
	return &DLError{
		name:       name,
		innerError: innerError,
	}
}
