package otg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("otg", New)
}

type config struct {
	Prefix     string `mapstructure:"prefix"`
	DbUsername string `mapstructure:"db_username"`
	DbPassword string `mapstructure:"db_password"`
	DbHost     string `mapstructure:"db_host"`
	DbPort     int    `mapstructure:"db_port"`
	DbName     string `mapstructure:"db_name"`
}

// New returns a new otg service
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}

	c.init()

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", c.DbUsername, c.DbPassword, c.DbHost, c.DbPort, c.DbName))
	if err != nil {
		return nil, err
	}

	return &svc{conf: c, db: db}, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return s.db.Close()
}

func (c *config) init() {
	if c.Prefix == "" {
		c.Prefix = "otg"
	}
}

type svc struct {
	conf *config
	db   *sql.DB
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Unprotected() []string {
	return nil
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			code := http.StatusMethodNotAllowed
			http.Error(w, http.StatusText(code), code)
			return
		}

		msg, err := s.getOTG(r.Context())
		if err != nil {
			var code int
			if errors.Is(err, sql.ErrNoRows) {
				code = http.StatusNoContent
			} else {
				code = http.StatusInternalServerError
			}
			http.Error(w, http.StatusText(code), code)
			return
		}

		encodeMessageAndSend(w, msg)
	})
}

func encodeMessageAndSend(w http.ResponseWriter, msg string) {
	res := struct {
		Message string `json:"message"`
	}{
		Message: msg,
	}
	data, err := json.Marshal(&res)
	if err != nil {
		code := http.StatusInternalServerError
		http.Error(w, http.StatusText(code), code)
		return
	}
	w.Write(data)
}

func (s *svc) getOTG(ctx context.Context) (string, error) {
	row := s.db.QueryRowContext(ctx, "SELECT message FROM cbox_otg_ocis")
	if row.Err() != nil {
		return "", row.Err()
	}

	var msg string
	if err := row.Scan(&msg); err != nil {
		return "", err
	}

	return msg, nil
}
