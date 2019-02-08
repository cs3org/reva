package auth

import (
	"fmt"
	"net/http"

	"github.com/cernbox/reva/pkg/log"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers"
	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/registry"

	"github.com/mitchellh/mapstructure"
)

var logger = log.New("auth")

type config struct {
	Strategy   string                            `mapstructure:"strategy"`
	Strategies map[string]map[string]interface{} `mapstructure:"strategies"`
}

// Register registers an auth handler.
func Register(m map[string]interface{}) error {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return err
	}

	f, ok := registry.NewFuncs[conf.Strategy]
	if !ok {
		return fmt.Errorf("auth strategy not found: %s", conf.Strategy)
	}

	s, err := f(conf.Strategies[conf.Strategy])
	if err != nil {
		return err
	}

	chain := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			creds, err := s.GetCredentials(r)
			if err != nil {
				logger.Error(r.Context(), err)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			logger.Println(r.Context(), creds)
			// TODO(labkode) create user context.
			h.ServeHTTP(w, r)
		})
	}

	handlers.Register("auth", chain)
	return nil
}
