package ocmd

import (
	"fmt"
	"github.com/rs/zerolog"
	"net/http"
)

func (s *svc) listAllShares(logger *zerolog.Logger) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()
		user := r.Header.Get("Remote-User")

		logger.Debug().Str("ctx", fmt.Sprintf("%+v", ctx)).Str("user", user).Msg("listAllShares")
		logger.Debug().Str("Variable: `s` type", fmt.Sprintf("%T", s)).Str("Variable: `s` value", fmt.Sprintf("%+v", s)).Msg("listAllShares")

		shares, err := s.GetShares(logger, ctx, user)

		logger.Debug().Str("err", fmt.Sprintf("%+v", err)).Str("shares", fmt.Sprintf("%+v", shares)).Msg("listAllShares")

		if err != nil {
			logger.Err(err).Msg("Error reading shares from manager")
			w.WriteHeader(http.StatusNotImplemented)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	})
}

func (s *svc) getShare(logger *zerolog.Logger, shareId string) http.Handler {
	return s.notImplemented(logger)
}
