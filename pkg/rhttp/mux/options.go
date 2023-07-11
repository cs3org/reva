package mux

import "strings"

type Options struct {
	Unprotected bool
	Middlewares []Middleware
}

type Option func(*Options)

func (o *Options) String() string {
	var b strings.Builder
	if o.Unprotected {
		b.WriteString("unprotected")
	}
	return b.String()
}

func (o *Options) merge(other *Options) *Options {
	if o == nil {
		return other
	}
	opt := *o
	if other == nil {
		return &opt
	}
	opt.Unprotected = opt.Unprotected || other.Unprotected
	opt.Middlewares = append(opt.Middlewares, other.Middlewares...)
	return &opt
}

func Unprotected() Option {
	return func(o *Options) {
		o.Unprotected = true
	}
}

func WithMiddleware(middleware Middleware) Option {
	return func(o *Options) {
		o.Middlewares = append(o.Middlewares, middleware)
	}
}
