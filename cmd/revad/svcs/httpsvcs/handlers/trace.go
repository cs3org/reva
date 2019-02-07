package handlers

import (
	"context"
	"net/http"

	"github.com/gofrs/uuid"
	"google.golang.org/grpc/metadata"
)

// TraceHandler is a middlware that checks if there is a trace provided
// as X-Trace header or generates one on the fly
// then the trace is stored in the context
func TraceHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var trace string
		val, ok := ctx.Value("trace").(string)
		if ok && val != "" {
			trace = val
		} else {
			// try to get it from header
			trace = r.Header.Get("x-trace")
			if trace == "" {
				trace = genTrace()
			}
		}

		ctx = context.WithValue(ctx, "trace", trace)
		header := metadata.New(map[string]string{"x-trace": trace})
		ctx = metadata.NewOutgoingContext(ctx, header)
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}

func genTrace() string {
	return uuid.Must(uuid.NewV4()).String()
}
