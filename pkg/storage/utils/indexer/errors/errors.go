package errors

import (
	"fmt"

	"github.com/cs3org/reva/pkg/storage/utils/indexer/option"
)

// AlreadyExistsErr implements the Error interface.
type AlreadyExistsErr struct {
	TypeName, Value string
	IndexBy         option.IndexBy
}

func (e *AlreadyExistsErr) Error() string {
	return fmt.Sprintf("%s with %s=%s does already exist", e.TypeName, e.IndexBy.String(), e.Value)
}

// IsAlreadyExistsErr checks whether an error is of type AlreadyExistsErr.
func IsAlreadyExistsErr(e error) bool {
	_, ok := e.(*AlreadyExistsErr)
	return ok
}

// NotFoundErr implements the Error interface.
type NotFoundErr struct {
	TypeName, Value string
	IndexBy         option.IndexBy
}

func (e *NotFoundErr) Error() string {
	return fmt.Sprintf("%s with %s=%s not found", e.TypeName, e.IndexBy.String(), e.Value)
}

// IsNotFoundErr checks whether an error is of type IsNotFoundErr.
func IsNotFoundErr(e error) bool {
	_, ok := e.(*NotFoundErr)
	return ok
}
