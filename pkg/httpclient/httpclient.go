package httpclient

import (
	//"io"
	"context"
	"io"
	"net/http"

	"github.com/cs3org/reva/pkg/trace"
	//"net/url"
)

// New creates an http.Client with a custom round tripper that adds tracing headers
// This client must be used in the reva codebase and usage of standard http.Client
func New() *http.Client {
	t := &injectTransport{rt: http.DefaultTransport}
	client := http.Client{
		Transport: t,
	}
	return &client
}

// NewWithRoundTripper works as New but wraps the rt argument with the custom round tripper
func NewWithRoundTripper(rt http.RoundTripper) *http.Client {
	t := &injectTransport{rt: rt}
	client := http.Client{
		Transport: t,
	}
	return &client
}

type injectTransport struct {
	rt http.RoundTripper
}

func (t injectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// assume the request context has not been populated with tracing information.
	ctx := r.Context()

	traceID := trace.Get(ctx)

	r.Header.Add("X-Trace-ID", traceID)

	tkn, ok := appctx.ContextGetToken(ctx)
	if ok {
		httpReq.Header.Set(appctx.TokenHeader, tkn)
	}

	return t.rt.RoundTrip(r)
}

// NewRequest creates an HTTP request that sets tracing headers
func NewRequestWithContext(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	return httpReq, nil
}
