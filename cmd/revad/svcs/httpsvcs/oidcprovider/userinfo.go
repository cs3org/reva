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

package oidcprovider

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth/manager/oidc"
)

type session struct {
	User string
}

// The same thing (valid oauth2 client) but for using the client credentials grant
var appClientConf = clientcredentials.Config{
	ClientID:     "reva",
	ClientSecret: "foobar",
	Scopes:       []string{"openid profile email"},
	TokenURL:     "http://localhost:9998/oauth2/token",
}

func (s *svc) doUserinfo(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	hdr := r.Header.Get("Authorization")
	token := strings.TrimPrefix(hdr, "Bearer ")
	if token == "" {
		// TODO make realm configurable or read it from forwarded for header
		// see https://github.com/stanvit/go-forwarded as middleware
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s"`, r.Host))
		return
	}

	resp, err := appClientConf.Client(
		context.Background(),
	).PostForm(
		strings.Replace(appClientConf.TokenURL, "token", "introspect", -1),
		url.Values{
			"token": []string{token},
			"scope": []string{r.URL.Query().Get("scope")},
		},
	)
	if err != nil {
		fmt.Fprintf(w, "<h1>An error occurred!</h1><p>Could not perform introspection request: %v</p>", err)
		return
	}
	defer resp.Body.Close()

	var introspection = &oidc.IntrospectionResponse{}
	out, _ := ioutil.ReadAll(resp.Body)
	if err := json.Unmarshal(out, &introspection); err != nil {
		log.Error().Err(err).Msg("error unmarshaling introspection claims")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !introspection.Active {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	log.Debug().Interface("claims", introspection).Msg("introspected")

	sc := &oidc.StandardClaims{
		Sub:               "a25cbd3c-f7f7-481d-a6f5-ec5983d88fa1",
		Email:             "aaliyah_adams@owncloudqa.com",
		EmailVerified:     true,
		Name:              "Aaliyah Adams",
		PreferredUsername: "aaliyah_adams",
	}
	b, err := json.Marshal(sc)
	if err != nil {
		log.Error().Err(err).Msg("error marshaling standard claims")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
