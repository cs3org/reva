

package ocssvc

import (
	"net/http"

	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
)

type AppsHandler struct {
	SharesHandler *SharesHandler
}

func (h *AppsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var head string
    head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
	switch head {
	case "files_sharing":
		head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
		if head == "api" {
			head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
			if head == "v1" {
				h.SharesHandler.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "Not Found", http.StatusNotFound)
	default:
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}