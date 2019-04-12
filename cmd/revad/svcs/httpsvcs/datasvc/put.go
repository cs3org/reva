package datasvc

import (
	"net/http"
	"strings"
)

func (s *svc) doPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fn := r.URL.Path

	fsfn := strings.TrimPrefix(fn, s.conf.ProviderPath)
	err := s.storage.Upload(ctx, fsfn, r.Body)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	r.Body.Close()
	w.WriteHeader(http.StatusOK)
}
