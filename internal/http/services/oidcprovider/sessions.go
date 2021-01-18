// Copyright 2018-2021 CERN
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
	"fmt"
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
)

func (s *svc) doSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	// Clients
	_, err := w.Write([]byte(`<p>Clients</p><ul>`))
	if err != nil {
		log.Error().Err(err).Msg("Error writing response")
		return
	}
	for id, c := range s.store.Clients {
		c := c
		_, err := w.Write([]byte(fmt.Sprintf(`
			<li>
				%s: Id %s, IsPublic %t, GetHashedSecret %s
			</li>`,
			id, c.GetID(), c.IsPublic(), c.GetHashedSecret(),
		)))
		if err != nil {
			log.Error().Err(err).Msg("Error writing response")
			return
		}
	}
	// AuthorizeCodes
	_, err = w.Write([]byte(`</ul><p>AuthorizeCodes</p><ul>`))
	if err != nil {
		log.Error().Err(err).Msg("Error writing response")
		return
	}
	for id, c := range s.store.AuthorizeCodes {
		c := c
		_, err := w.Write([]byte(fmt.Sprintf(`
			<li>
				%s: %#v
			</li>`,

			id, c,
		)))
		if err != nil {
			log.Error().Err(err).Msg("Error writing response")
			return
		}
	}
	// IDSessions
	_, err = w.Write([]byte(`</ul><p>IDSessions</p><ul>`))
	if err != nil {
		log.Error().Err(err).Msg("Error writing response")
		return
	}
	for id, c := range s.store.IDSessions {
		c := c
		_, err := w.Write([]byte(fmt.Sprintf(`
			<li>
				%s: %#v
			</li>`,

			id, c,
		)))
		if err != nil {
			log.Error().Err(err).Msg("Error writing response")
			return
		}
	}
	// AccessTokens
	_, err = w.Write([]byte(`</ul><p>AccessTokens</p><ul>`))
	if err != nil {
		log.Error().Err(err).Msg("Error writing response")
		return
	}
	for id, c := range s.store.AccessTokens {
		_, err := w.Write([]byte(fmt.Sprintf(`
			<li>
				%s: %#v
			</li>`,

			id, c,
		)))
		if err != nil {
			log.Error().Err(err).Msg("Error writing response")
			return
		}
	}
	// RefreshTokens
	_, err = w.Write([]byte(`</ul><p>RefreshTokens</p><ul>`))
	if err != nil {
		log.Error().Err(err).Msg("Error writing response")
		return
	}
	for id, c := range s.store.RefreshTokens {
		c := c
		_, err := w.Write([]byte(fmt.Sprintf(`
			<li>
				%s: %#v
			</li>`,

			id, c,
		)))
		if err != nil {
			log.Error().Err(err).Msg("Error writing response")
			return
		}
	}
	// PKCES
	_, err = w.Write([]byte(`</ul><p>PKCES</p><ul>`))
	if err != nil {
		log.Error().Err(err).Msg("Error writing response")
		return
	}
	for id, c := range s.store.PKCES {
		c := c
		_, err := w.Write([]byte(fmt.Sprintf(`
			<li>
				%s: %#v
			</li>`,

			id, c,
		)))
		if err != nil {
			log.Error().Err(err).Msg("Error writing response")
			return
		}
	}
	// AccessTokenRequestIDs
	_, err = w.Write([]byte(`</ul><p>AccessTokenRequestIDs</p><ul>`))
	if err != nil {
		log.Error().Err(err).Msg("Error writing response")
		return
	}
	for id, c := range s.store.AccessTokenRequestIDs {
		c := c
		_, err := w.Write([]byte(fmt.Sprintf(`
			<li>
				%s: %#v
			</li>`,

			id, c,
		)))
		if err != nil {
			log.Error().Err(err).Msg("Error writing response")
			return
		}
	}
	// RefreshTokenRequestIDs
	_, err = w.Write([]byte(`</ul><p>RefreshTokenRequestIDs</p><ul>`))
	if err != nil {
		log.Error().Err(err).Msg("Error writing response")
		return
	}
	for id, c := range s.store.RefreshTokenRequestIDs {
		c := c
		_, err := w.Write([]byte(fmt.Sprintf(`
			<li>
				%s: %#v
			</li>`,

			id, c,
		)))
		if err != nil {
			log.Error().Err(err).Msg("Error writing response")
			return
		}
	}
	_, err = w.Write([]byte(`</ul>`))
	if err != nil {
		log.Error().Err(err).Msg("Error writing response")
	}
}
