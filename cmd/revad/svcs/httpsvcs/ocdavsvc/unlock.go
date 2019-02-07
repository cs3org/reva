package ocdavsvc

import (
	"net/http"
)

func (s *svc) doUnlock(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
