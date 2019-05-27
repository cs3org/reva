

package ocssvc

import (
	"net/http"

	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
)

type V1Handler struct {
	AppsHandler *AppsHandler
	CloudHandler *CloudHandler
	ConfigHandler *ConfigHandler
}

func (h *V1Handler) init(c *Config) {
	h.AppsHandler = new(AppsHandler)
	h.CloudHandler = new(CloudHandler)
	h.CloudHandler.init(c)
	h.ConfigHandler = new(ConfigHandler)
	h.ConfigHandler.init(c)
}

func (h *V1Handler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var head string
		head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
		switch head {
		case "apps":
			h.AppsHandler.ServeHTTP(w, r)
		case "cloud":
			h.CloudHandler.Handler().ServeHTTP(w, r)
		case "config":
			h.ConfigHandler.Handler().ServeHTTP(w, r)
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	})
}