

package ocssvc

import (
	"net/http"

	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
)

type CloudHandler struct {
	UserHandler *UserHandler
	CapabilitiesHandler *CapabilitiesHandler
}

func (h *CloudHandler) init(c *Config) {
	h.UserHandler = new(UserHandler)
	h.CapabilitiesHandler = new(CapabilitiesHandler)
	h.CapabilitiesHandler.init(c)
}

func (h *CloudHandler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var head string
		head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
		switch head {
		case "capabilities":
			h.CapabilitiesHandler.Handler().ServeHTTP(w, r)
		case "user":
			h.UserHandler.ServeHTTP(w, r)
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	})
}