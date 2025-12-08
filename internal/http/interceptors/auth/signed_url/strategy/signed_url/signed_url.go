// Copyright 2018-2025 CERN
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

package basic

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/interceptors/auth/signed_url/registry"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/auth/signing"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/crypto/pbkdf2"
)

func init() {
	registry.Register("signed_url", New)
}

type Config struct {
	// PreSignedURL is the config for the pre-signed url interceptor
	AllowedHTTPMethods   []string `mapstructure:"allowed_http_methods"`
	Enabled              bool     `mapstructure:"enabled"`
	SigningKeySecret     string   `mapstructure:"signing_key_secret"`
	UserProviderEndpoint string   `mapstructure:"userprovidersvc"`
	// Default: one day
	MaxExpirySeconds int `mapstructure:"max_expiry_seconds" docs:"nil; Default: one day"`
}

type SignedURLAuthenticator struct {
	config *Config
}

func (c *Config) ApplyDefaults() {
	if c.MaxExpirySeconds == 0 {
		c.MaxExpirySeconds = 24 * 60 * 60
	}
}

// New returns a new auth strategy that checks for signed URLs.
func New(m map[string]any) (auth.SignedURLStrategy, error) {
	c := &Config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	c.ApplyDefaults()

	return &SignedURLAuthenticator{config: c}, nil
}

const (
	// OC-Signature: the computed signature - server will verify the request upon this REQUIRED
	_paramOCSignature = "OC-Signature"
	// OC-Credential: defines the user scope (shall we use the owncloud user id here - this might leak internal data ....) REQUIRED
	_paramOCCredential = "OC-Credential"
	// OC-Date: defined the date the url was signed (ISO 8601 UTC) REQUIRED
	_paramOCDate = "OC-Date"
	// OC-Expires: defines the expiry interval in seconds (between 1 and 604800 = 7 days) REQUIRED
	_paramOCExpires = "OC-Expires"
	// OC-Verb: defines for which http verb the request is valid - defaults to GET OPTIONAL
	_paramOCVerb = "OC-Verb"
)

var (
	_requiredParamsToSign = []string{
		_paramOCCredential,
		_paramOCDate,
		_paramOCExpires,
		_paramOCVerb,
	}
	_requiredParams = append(_requiredParamsToSign, _paramOCSignature)
)

func (m SignedURLAuthenticator) shouldServe(req *http.Request) bool {
	if !m.config.Enabled {
		return false
	}
	return req.URL.Query().Get(_paramOCSignature) != ""
}

func (m SignedURLAuthenticator) validate(req *http.Request) (err error) {
	query := req.URL.Query()

	if err := m.allRequiredParametersArePresent(query); err != nil {
		return err
	}

	if err := m.requestMethodIsAllowed(req.Method); err != nil {
		return err
	}

	if err = m.urlIsExpired(query); err != nil {
		return err
	}

	if err := m.signatureIsValid(req); err != nil {
		return err
	}

	return nil
}

// check if required query parameters exist in given request query parameters
func (m SignedURLAuthenticator) allRequiredParametersArePresent(query url.Values) (err error) {
	for _, p := range _requiredParams {
		if query.Get(p) == "" {
			return fmt.Errorf("required %s parameter not found", p)
		}
	}
	return nil
}

// check if given request method is allowed according to the config
func (m SignedURLAuthenticator) requestMethodIsAllowed(method string) (err error) {
	methodIsAllowed := false
	for _, am := range m.config.AllowedHTTPMethods {
		if strings.EqualFold(method, am) {
			methodIsAllowed = true
			break
		}
	}

	if !methodIsAllowed {
		return errors.New("request method is not listed in PreSignedURLConfig AllowedHTTPMethods")
	}

	return nil
}

// check if url is expired by checking if given date (OC-Date) + expires in seconds (OC-Expires) is after now
func (m SignedURLAuthenticator) urlIsExpired(query url.Values) (err error) {
	validFrom, err := time.Parse(time.RFC3339, query.Get(_paramOCDate))
	if err != nil {
		return err
	}

	requestExpiry, err := time.ParseDuration(query.Get(_paramOCExpires) + "s")
	if err != nil {
		return err
	}

	maxDuration := time.Duration(m.config.MaxExpirySeconds) * time.Second
	if requestExpiry > maxDuration {
		return errors.New("expiry invalid")
	}

	validTo := validFrom.Add(requestExpiry)
	if !(time.Now().Before(validTo)) {
		return errors.New("URL is expired")
	}

	return nil
}

func (m SignedURLAuthenticator) signatureIsValid(req *http.Request) (err error) {
	ctx := req.Context()
	// The signed URL contains the requested credential (under "OC-Credential"),
	// this is parsed before and placed in the context
	userFromCtx := appctx.ContextMustGetUser(ctx)
	date := req.URL.Query().Get(_paramOCDate)
	signingKey := signing.DeriveSigningKey(userFromCtx, m.config.SigningKeySecret, date)

	u := m.buildUrlToSign(req)

	computedSignature := m.createSignature(u, signingKey)
	signatureInURL := req.URL.Query().Get(_paramOCSignature)
	if computedSignature == signatureInURL {
		return nil
	}

	// try a workaround for https://github.com/owncloud/ocis/issues/10180
	// Some reverse proxies might replace $ with %24 in the URL leading to a mismatch in the signature
	u = strings.Replace(u, "$", "%24", 1)
	u = strings.Replace(u, "=", "%3D", 3)
	computedSignature = m.createSignature(u, signingKey)
	signatureInURL = req.URL.Query().Get(_paramOCSignature)
	if computedSignature == signatureInURL {
		return nil
	}

	return fmt.Errorf("signature mismatch: expected %s != actual %s", computedSignature, signatureInURL)
}

func (m SignedURLAuthenticator) buildUrlToSign(req *http.Request) string {
	q := req.URL.Query()

	// We only take into account params required for signing
	signParameters := make(url.Values)
	for _, p := range _requiredParamsToSign {
		signParameters.Add(p, q.Get(p))
		q.Del(p)
	}

	// Now let's remove any remaining query params that are not part of the URL that is to be signed
	for qParam, _ := range req.URL.Query() {
		q.Del(qParam)
	}

	urlToSign := *req.URL
	if len(q) == 0 {
		urlToSign.RawQuery = signParameters.Encode()
	} else {
		urlToSign.RawQuery = strings.Join([]string{q.Encode(), signParameters.Encode()}, "&")
	}
	u := urlToSign.String()
	if !urlToSign.IsAbs() {
		u = "https://" + req.Host + u
	}
	return u
}

func (m SignedURLAuthenticator) createSignature(url string, signingKey []byte) string {
	// the oc10 signature check: $hash = \hash_pbkdf2("sha512", $url, $signingKey, 10000, 64, false);
	// - sets the length of the output string to 64
	// - sets raw output to false ->  if raw_output is FALSE length corresponds to twice the byte-length of the derived key (as every byte of the key is returned as two hexits).
	// TODO change to length 128 in oc10?
	// fo golangs pbkdf2.Key we need to use 32 because it will be encoded into 64 hexits later
	hash := pbkdf2.Key([]byte(url), signingKey, 10000, 32, sha512.New)
	return hex.EncodeToString(hash)
}

// Authenticate implements the authenticator interface to authenticate requests via signed URL auth.
func (m SignedURLAuthenticator) Authenticate(r *http.Request) (*http.Request, bool) {
	ctx := r.Context()
	sublog := appctx.GetLogger(ctx).With().Str("authenticator", "signed_url").Str("path", r.URL.Path).Logger()

	client, err := pool.GetUserProviderServiceClient(pool.Endpoint(m.config.UserProviderEndpoint))
	if err != nil {
		return nil, false
	}

	if !m.shouldServe(r) {
		return nil, false
	}

	userResp, err := client.GetUserByClaim(ctx, &user.GetUserByClaimRequest{
		Claim: "username",
		Value: r.URL.Query().Get(_paramOCCredential),
	})
	if err != nil {
		sublog.Error().Err(err).Msg("Could not get user by claim")
		return nil, false
	}
	if userResp.Status.Code != rpcv1beta1.Code_CODE_OK {
		sublog.Error().Str("Status", userResp.Status.String()).Msg("Could not get user by claim")
		return nil, false
	}

	userCtx := appctx.ContextSetUser(r.Context(), userResp.User)

	r = r.WithContext(userCtx)

	if err := m.validate(r); err != nil {
		sublog.Error().Err(err).Str("url", r.URL.String()).Msg("Could not get user by claim")
		return nil, false
	}

	sublog.Debug().Msg("successfully authenticated request")
	return r, true
}
