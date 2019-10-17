// Copyright 2018-2019 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package datagateway

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/dgrijalva/jwt-go"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const (
	tokenTransportHeader = "X-Reva-Transfer"
)

func init() {
	rhttp.Register("datagateway", New)
}

// transerClaims are custom claims for a JWT token to be used between the metadata and data gateways.
type transferClaims struct {
	jwt.StandardClaims
	Target string `json:"target"`
}
type config struct {
	Prefix               string `mapstructure:"prefix"`
	GatewayEndpoint      string `mapstructure:"gateway"`
	TransferSharedSecret string `mapstructure:"transfer_shared_secret"`
}

type svc struct {
	conf    *config
	handler http.Handler
}

// New returns a new datagateway
func New(m map[string]interface{}) (rhttp.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	s := &svc{conf: conf}
	s.setHandler()
	return s, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}

func (s *svc) setHandler() {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "HEAD":
			addCorsHeader(w)
			w.WriteHeader(http.StatusOK)
			return
		case "GET":
			s.doGet(w, r)
			return
		case "PUT":
			s.doPut(w, r)
			return
		default:
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
	})
}

func addCorsHeader(res http.ResponseWriter) {
	headers := res.Header()
	headers.Set("Access-Control-Allow-Origin", "*")
	headers.Set("Access-Control-Allow-Headers", "Content-Type, Origin, Authorization")
	headers.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD")
}

func (s *svc) verify(ctx context.Context, token string) (*transferClaims, error) {
	j, err := jwt.ParseWithClaims(token, &transferClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.conf.TransferSharedSecret), nil
	})

	if err != nil {
		return nil, errors.Wrap(err, "error parsing token")
	}

	if claims, ok := j.Claims.(*transferClaims); ok && j.Valid {
		return claims, nil
	}

	err = errtypes.InvalidCredentials("token invalid")
	return nil, err
}

func (s *svc) doGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	token := r.Header.Get(tokenTransportHeader)
	claims, err := s.verify(ctx, token)
	if err != nil {
		err = errors.Wrap(err, "datagateway: error validating transfer token")
		log.Err(err)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	log.Info().Str("target", claims.Target).Msg("sending request to internal data server")

	httpClient := rhttp.GetHTTPClient(ctx)
	httpReq, err := rhttp.NewRequest(ctx, "GET", claims.Target, nil)
	if err != nil {
		log.Err(err).Msg("wrong request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		log.Err(err).Msg("error doing GET request to data service")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if httpRes.StatusCode != http.StatusOK {
		w.WriteHeader(httpRes.StatusCode)
		return
	}

	defer httpRes.Body.Close()
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, httpRes.Body)
	if err != nil {
		log.Err(err).Msg("error writing body after headers were sent")
	}
}

func (s *svc) doPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	token := r.Header.Get(tokenTransportHeader)
	claims, err := s.verify(ctx, token)
	if err != nil {
		err = errors.Wrap(err, "datagateway: error validating transfer token")
		log.Err(err).Str("token", token).Msg("invalid token")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	target := claims.Target
	// add query params to target, clients can send checksums and other information.
	targetURL, err := url.Parse(target)
	if err != nil {
		log.Err(err).Msg("datagateway: error parsing target url")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	targetURL.RawQuery = r.URL.RawQuery
	target = targetURL.String()

	log.Info().Str("target", claims.Target).Msg("sending request to internal data server")

	httpClient := rhttp.GetHTTPClient(ctx)
	httpReq, err := rhttp.NewRequest(ctx, "PUT", target, r.Body)
	if err != nil {
		log.Err(err).Msg("wrong request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		log.Err(err).Msg("error doing PUT request to data service")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if httpRes.StatusCode != http.StatusOK {
		w.WriteHeader(httpRes.StatusCode)
		return
	}

	defer httpRes.Body.Close()
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, httpRes.Body)
	if err != nil {
		log.Err(err).Msg("error writing body after header were set")
	}
}
