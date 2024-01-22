package secure

import (
	"net/http"

	"github.com/cs3org/reva/v2/pkg/rhttp/global"
	"github.com/mitchellh/mapstructure"
)

const (
	defaultPriority = 200
)

func init() {
	global.RegisterMiddleware("secure", New)
}

type secure struct {
	ContentSecurityPolicy string `mapstructure:"content_security_policy"`
	Priority              int    `mapstructure:"priority"`
}

// New creates a new secure middleware.
func New(m map[string]interface{}) (global.Middleware, int, error) {
	s := &secure{}
	if err := mapstructure.Decode(m, s); err != nil {
		return nil, 0, err
	}

	if s.Priority == 0 {
		s.Priority = defaultPriority
	}

	if s.ContentSecurityPolicy == "" {
		s.ContentSecurityPolicy = "frame-ancestors 'none'"
	}

	return s.Handler, s.Priority, nil
}

// Handler is the middleware function.
func (m *secure) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Indicates whether the browser is allowed to render this page in a <frame>, <iframe>, <embed> or <object>.
		w.Header().Set("X-Frame-Options", "DENY")
		// Does basically the same as X-Frame-Options.
		w.Header().Set("Content-Security-Policy", m.ContentSecurityPolicy)
		// This header inidicates that MIME types advertised in the Content-Type headers should not be changed and be followed.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Disallow iFraming from other domains
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		// https://msdn.microsoft.com/en-us/library/jj542450(v=vs.85).aspx
		w.Header().Set("X-Download-Options", "noopen")
		// Disallow iFraming from other domains
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		// https://www.adobe.com/devnet/adobe-media-server/articles/cross-domain-xml-for-streaming.html
		w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")
		// https://developers.google.com/webmasters/control-crawl-index/docs/robots_meta_tag
		w.Header().Set("X-Robots-Tag", "none")
		// enforce browser based XSS filters
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		if r.TLS != nil {
			// Tell browsers that the website should only be accessed  using HTTPS.
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}

		next.ServeHTTP(w, r)
	})
}
