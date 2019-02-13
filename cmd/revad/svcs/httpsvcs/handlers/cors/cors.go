package cors

import (
	"github.com/cernbox/reva/pkg/log"

	"github.com/cernbox/reva/cmd/revad/httpserver"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/cors"
)

func init() {
	httpserver.RegisterMiddleware("cors", New)
}

var logger = log.New("cors")

type config struct {
	Priority           int      `mapstructure:"priority"`
	AllowedOrigins     []string `mapstructure:"allowed_origins"`
	AllowCredentials   bool     `mapstructure:"allow_credentials"`
	AllowedMethods     []string `mapstructure:"allowed_methods"`
	AllowedHeaders     []string `mapstructure:"allowed_headers"`
	ExposedHeaders     []string `mapstructure:"exposed_headers"`
	MaxAge             int      `mapstructure:"max_age"`
	OptionsPassthrough bool     `mapstructure:"options_passthrough"`
}

// New creates a new CORS middleware.
func New(m map[string]interface{}) (httpserver.Middleware, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}

	c := cors.New(cors.Options{
		AllowCredentials:   conf.AllowCredentials,
		AllowedHeaders:     conf.AllowedHeaders,
		AllowedMethods:     conf.AllowedMethods,
		AllowedOrigins:     conf.AllowedOrigins,
		ExposedHeaders:     conf.ExposedHeaders,
		MaxAge:             conf.MaxAge,
		OptionsPassthrough: conf.OptionsPassthrough,
	})

	return c.Handler, conf.Priority, nil
}
