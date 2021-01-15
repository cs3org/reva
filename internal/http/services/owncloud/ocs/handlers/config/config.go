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

package ocs

import (
	"net/http"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/data"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
)

// Handler renders the config endpoint
type Handler struct {
	c data.ConfigData
}

// Init initializes this and any contained handlers
func (h *Handler) Init(c *config.Config) {
	h.c = c.Config
	// config
	if h.c.Version == "" {
		h.c.Version = "1.7"
	}
	if h.c.Website == "" {
		h.c.Website = "reva"
	}
	if h.c.Host == "" {
		h.c.Host = "" // TODO get from context?
	}
	if h.c.Contact == "" {
		h.c.Contact = ""
	}
	if h.c.SSL == "" {
		h.c.SSL = "false" // TODO get from context?
	}
}

// Handler renders the config
func (h *Handler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.WriteOCSSuccess(w, r, h.c)
	})
}
