

package ocssvc

import (
	"net/http"

	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
)

type SharesHandler struct {
}

func (h *SharesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var head string
    head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
	switch head {
	case "shares":
		h.doShares(w, r)
	default:
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

func (h *SharesHandler) doShares(w http.ResponseWriter, r *http.Request) {
	res := &Response{
		OCS: &Payload{
			Meta: MetaOK,
			Data: SharesData{
				Shares: []*ShareData{
					&ShareData{ ID: "1", },
					&ShareData{ ID: "2", },
				},
			},
		},
	}

	err := WriteOCSResponse(w, r, res)
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing ocs response")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// SharesData holds a list of share data
type SharesData struct {
	Shares []*ShareData `json:"element" xml:"element"`
}


// ShareData holds share data
type ShareData struct {
	ID string `json:"id" xml:"id"`
}
