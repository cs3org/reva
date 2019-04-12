package datasvc

import (
	"io"
	"net/http"
	"strings"
)

func (s *svc) doGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fn := r.URL.Path

	fsfn := strings.TrimPrefix(fn, s.conf.ProviderPath)
	rc, err := s.storage.Download(ctx, fsfn)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = io.Copy(w, rc)
	if err != nil {
		logger.Error(ctx, err)
		return
	}
}
