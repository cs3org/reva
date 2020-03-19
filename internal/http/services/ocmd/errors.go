package ocmd

import (
	"github.com/rs/zerolog"
	"net/http"
)

func (s *svc) notImplemented(logger *zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		apiErr := newAPIError(apiErrorUnimplemented)
		if _, err := w.Write(apiErr.JSON()); err != nil {
			logger.Err(err).Msg("Error writing to ResponseWriter")
		}
	})
}

func (s *svc) methodNotAllowed(logger *zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		apiErr := newAPIError(apiErrorMethodNotAllowed)
		if _, err := w.Write(apiErr.JSON()); err != nil {
			logger.Err(err).Msg("Error writing to ResponseWriter")
		}
	})
}
