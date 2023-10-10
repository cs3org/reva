package trace

import (
	"context"
	"net/http"
	"testing"

	"github.com/cs3org/reva/pkg/trace"
)

type testPair struct {
	e string
	r *http.Request
}

func TestGetTrace(t *testing.T) {
	pairs := []*testPair{
		&testPair{
			r: newRequest(context.Background(), map[string]string{"X-Trace-ID": "def"}),
			e: "def",
		},
		&testPair{
			r: newRequest(context.Background(), map[string]string{"X-Request-ID": "abc"}),
			e: "abc",
		},
		&testPair{
			r: newRequest(trace.Set(context.Background(), "fgh"), nil),
			e: "fgh",
		},
	}

	for _, p := range pairs {
		got, _ := getTraceID(p.r)
		t.Logf("headers: %+v context: %+v got: %+v\n", p.r, p.r.Context(), got)
		if got != p.e {
			t.Fatal("expected: "+p.e, "got: "+got)
			return
		}
	}

}

func newRequest(ctx context.Context, headers map[string]string) *http.Request {
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestGenerateTrace(t *testing.T) {
	got, _ := getTraceID(newRequest(context.Background(), nil))
	if len(got) != 36 {
		t.Fatal("expected random generated UUID 36 chars trace ID but got:" + got)
		return
	}

}
