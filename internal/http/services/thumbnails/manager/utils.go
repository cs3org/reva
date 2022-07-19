package manager

import "io"

type funcCloser struct {
	f func() error
	r io.Reader
}

// NewFuncReadCloser create a ReadCloser that on close run the function f
func NewFuncReadCloser(r io.Reader, f func() error) io.ReadCloser {
	return &funcCloser{
		r: r,
		f: f,
	}
}

func (f *funcCloser) Close() error {
	return f.f()
}

func (f *funcCloser) Read(data []byte) (n int, err error) {
	return f.r.Read(data)
}
