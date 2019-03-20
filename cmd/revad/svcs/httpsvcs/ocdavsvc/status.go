package ocdavsvc

import (
	"encoding/json"
	"net/http"
)

func (s *svc) doStatus(w http.ResponseWriter, r *http.Request) {
	status := &ocsStatus{
		Installed:      true,
		Maintenance:    false,
		NeedsDBUpgrade: false,
		Version:        "10.0.9.5",  // TODO make build determined
		VersionString:  "10.0.9",    // TODO make build determined
		Edition:        "community", // TODO make build determined
		ProductName:    "ownCloud",  // TODO make configurable
	}

	statusJSON, err := json.MarshalIndent(status, "", "    ")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(statusJSON)
}
