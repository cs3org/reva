package ocmd

import (
	"github.com/rs/zerolog"
	"net/http"
)

func (s *svc) listAllShares(logger *zerolog.Logger, sm shareManager, pa providerAuthorizer) http.Handler {
	return s.notImplemented(logger)
}

func (s *svc) getShare(logger *zerolog.Logger, sm shareManager, pa providerAuthorizer, shareId string) http.Handler {
	return s.notImplemented(logger)
}
