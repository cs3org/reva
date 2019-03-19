package ocdavsvc

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
)

type ocsResponse struct {
	OCS *ocsPayload `json:"ocs"`
}

type ocsPayload struct {
	XMLName struct{}         `json:"-" xml:"ocs"`
	Meta    *ocsResponseMeta `json:"meta" xml:"meta"`
	Data    interface{}      `json:"data,omitempty" xml:"data,omitempty"`
}

type ocsResponseMeta struct {
	Status       string `json:"status" xml:"status"`
	StatusCode   int    `json:"statuscode" xml:"statuscode"`
	Message      string `json:"message" xml:"message"`
	TotalItems   string `json:"totalitems,omitempty" xml:"totalitems,omitempty"`
	ItemsPerPage string `json:"itemsperpage,omitempty" xml:"itemsperpage,omitempty"`
}

var ocsMetaOK = &ocsResponseMeta{Status: "ok", StatusCode: 100, Message: "OK"}

type ocsUserData struct {
	// TODO needs better naming, clarify if we need a userid, a username or both
	ID          string `json:"id" xml:"id"`
	DisplayName string `json:"display-name" xml:"display-name"`
	Email       string `json:"email" xml:"email"`
}

type ocsConfigData struct {
	Version string `json:"version" xml:"version"`
	Website string `json:"website" xml:"website"`
	Host    string `json:"host" xml:"host"`
	Contact string `json:"contact" xml:"contact"`
	SSL     string `json:"ssl" xml:"ssl"`
}

// handles writing ocs responses in json and xml
func writeOCSResponse(w http.ResponseWriter, r *http.Request, res *ocsResponse) {
	ctx := r.Context()

	var encoded []byte
	var err error
	if r.URL.Query().Get("format") == "xml" {
		w.Write([]byte(xml.Header))
		encoded, err = xml.Marshal(res.OCS)
		w.Header().Set("Content-Type", "application/xml")
	} else {
		encoded, err = json.Marshal(res)
		w.Header().Set("Content-Type", "application/json")
	}
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(encoded)
}
