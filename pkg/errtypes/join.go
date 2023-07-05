package errtypes

import (
	"strings"
)

type joinErrors []error

func Join(err ...error) error {
	return joinErrors(err)
}

func (e joinErrors) Error() string {
	var b strings.Builder
	for i, err := range e {
		b.WriteString(err.Error())
		if i != len(e)-1 {
			b.WriteString(", ")
		}
	}
	return b.String()
}
