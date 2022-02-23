package option

import (
	"github.com/cs3org/reva/pkg/storage/utils/metadata"
)

// Option defines a single option function.
type Option func(o *Options)

// IndexBy defines how the data is being indexed
type IndexBy interface {
	String() string
}

// IndexByField represents the field that's being used to index the data by
type IndexByField string

// String returns a string representation
func (ibf IndexByField) String() string {
	return string(ibf)
}

// IndexByFunc represents a function that's being used to index the data by
type IndexByFunc struct {
	Name string
	Func func(v interface{}) (string, error)
}

// String returns a string representation
func (ibf IndexByFunc) String() string {
	return ibf.Name
}

// Bound represents a lower and upper bound range for an index.
// todo: if we would like to provide an upper bound then we would need to deal with ranges, in which case this is why the
// upper bound attribute is here.
type Bound struct {
	Lower, Upper int64
}

// Options defines the available options for this package.
type Options struct {
	CaseInsensitive bool
	Bound           *Bound

	TypeName string
	IndexBy  IndexBy
	FilesDir string
	Prefix   string

	Storage metadata.Storage
}

// CaseInsensitive sets the CaseInsensitive field.
func CaseInsensitive(val bool) Option {
	return func(o *Options) {
		o.CaseInsensitive = val
	}
}

// WithBounds sets the Bounds field.
func WithBounds(val *Bound) Option {
	return func(o *Options) {
		o.Bound = val
	}
}

// WithTypeName sets the TypeName option.
func WithTypeName(val string) Option {
	return func(o *Options) {
		o.TypeName = val
	}
}

// WithIndexBy sets the option IndexBy.
func WithIndexBy(val IndexBy) Option {
	return func(o *Options) {
		o.IndexBy = val
	}
}

// WithFilesDir sets the option FilesDir.
func WithFilesDir(val string) Option {
	return func(o *Options) {
		o.FilesDir = val
	}
}
