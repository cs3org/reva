package trace

import (
	"net/http"

	"github.com/cernbox/reva/cmd/revad/httpserver"
	tracepkg "github.com/cernbox/reva/pkg/trace"
	"github.com/gofrs/uuid"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc/metadata"
)

type config struct {
	Priority int    `mapstructure:"priority"`
	Header   string `mapstructure:"header"`
}

func init() {
	httpserver.RegisterMiddleware("trace", New)
}

// New returns a middleware that checks if there is a trace provided
// as X-Trace header or generates one on the fly
// then the trace is stored in the context.
func New(m map[string]interface{}) (httpserver.Middleware, int, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, 0, err
	}

	chain := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			var trace string
			val, ok := tracepkg.ContextGetTrace(ctx)
			if ok && val != "" {
				trace = val
			} else {
				// try to get it from header
				trace = r.Header.Get(c.Header)
				if trace == "" {
					trace = genTrace()
				}
			}

			ctx = tracepkg.ContextSetTrace(ctx, trace)
			header := metadata.New(map[string]string{c.Header: trace})
			ctx = metadata.NewOutgoingContext(ctx, header)
			r = r.WithContext(ctx)
			h.ServeHTTP(w, r)
		})
	}
	return chain, c.Priority, nil
}

func genTrace() string {
	return uuid.Must(uuid.NewV4()).String()
}
