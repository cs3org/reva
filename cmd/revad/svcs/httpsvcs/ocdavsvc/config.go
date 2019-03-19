package ocdavsvc

import (
	"net/http"
)

// the config for the ocs api
func (s *svc) doConfig(w http.ResponseWriter, r *http.Request) {
	res := &ocsResponse{
		OCS: &ocsPayload{
			Meta: ocsMetaOK,
			Data: &ocsConfigData{
				// hardcoded in core as well https://github.com/owncloud/core/blob/5f0af496626b957aff38730b5771ec0a33effe31/lib/private/OCS/Config.php#L28-L34
				Version: "1.7",
				Website: "ownCloud",
				Host:    r.URL.Host, // FIXME r.URL.Host is empty
				Contact: "",
				SSL:     "false",
			},
		},
	}
	writeOCSResponse(w, r, res)
}
