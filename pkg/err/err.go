package err

import (
	"github.com/pkg/errors"
)

type Err struct {
	prefix string
}

func New(prefix string) *Err {
	return &Err{prefix: prefix}
}

func (e *Err) Wrap(err error, msg string) error {
	msg = e.build(msg)
	return errors.Wrap(err, msg)
}

func (e *Err) build(msg string) string {
	return e.prefix + ": " + msg
}

func (e *Err) Wrapf(err error, format string, args ...interface{}) error {
	format = e.build(format)
	return errors.Wrapf(err, format, args...)
}

func (e *Err) Cause(err error) error {
	return errors.Cause(err)
}

func (e *Err) New(msg string) error {
	msg = e.build(msg)
	return errors.New(msg)
}

func (e *Err) Errorf(format string, args ...interface{}) error {
	format = e.build(format)
	return errors.Errorf(format, args...)
}
