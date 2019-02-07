package ocdavsvc

import (
	"net/http"
)

func (s *svc) doProppatch(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
