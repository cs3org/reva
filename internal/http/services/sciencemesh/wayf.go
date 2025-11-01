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

package sciencemesh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"
)

type wayfHandler struct {
	federations []Federation
	ocmClient   *ocmd.OCMClient
}

type Federation struct {
	Federation string             `json:"federation"`
	Servers    []FederationServer `json:"servers"`
}

// FederationServer represents a single provider with discovery info
type FederationServer struct {
	DisplayName        string `json:"displayName"`
	URL                string `json:"url"`
	InviteAcceptDialog string `json:"inviteAcceptDialog,omitempty"`
}

// federationFile is the on-disk structure without inviteAcceptDialog
type federationFile struct {
	Federation string                 `json:"federation"`
	Servers    []federationServerFile `json:"servers"`
}

type federationServerFile struct {
	DisplayName string `json:"displayName"`
	URL         string `json:"url"`
}

type DiscoverRequest struct {
	Domain string `json:"domain"`
}

type DiscoverResponse struct {
	InviteAcceptDialog string `json:"inviteAcceptDialog"`
}

func (h *wayfHandler) init(c *config) error {
	log := appctx.GetLogger(context.Background())

	// Create OCM client for discovery from config
	h.ocmClient = ocmd.NewClient(time.Duration(c.OCMClientTimeout)*time.Second, c.OCMClientInsecure)
	log.Debug().
		Int("timeout_seconds", c.OCMClientTimeout).
		Bool("insecure", c.OCMClientInsecure).
		Msg("Created OCM client for discovery")

	log.Debug().Str("file", c.FederationsFile).Msg("Initializing WAYF handler with federations file")

	data, err := os.ReadFile(c.FederationsFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn().Str("file", c.FederationsFile).Msg("Federations file not found, starting with empty list")
			h.federations = []Federation{}
			return nil
		}
		log.Error().Err(err).Str("file", c.FederationsFile).Msg("Failed to read federations file")
		return err
	}

	var fileData []federationFile
	if err := json.Unmarshal(data, &fileData); err != nil {
		log.Error().Err(err).Str("file", c.FederationsFile).Msg("Failed to parse federations file")
		return err
	}

	log.Debug().Int("federations_count", len(fileData)).Msg("Loaded federations from file")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Discover each server and populate inviteAcceptDialog
	h.federations = []Federation{}
	discoveryErrors := 0
	validServersCount := 0

	for _, fed := range fileData {
		log.Debug().Str("federation", fed.Federation).Int("servers_count", len(fed.Servers)).Msg("Processing federation")
		var validServers []FederationServer

		for _, srv := range fed.Servers {
			if srv.DisplayName == "" || srv.URL == "" {
				log.Debug().Str("federation", fed.Federation).
					Str("displayName", srv.DisplayName).
					Str("url", srv.URL).
					Msg("Skipping server with missing displayName or url")
				continue
			}

			log.Debug().Str("federation", fed.Federation).Str("server", srv.DisplayName).Str("url", srv.URL).Msg("Discovering server")

			// Discover inviteAcceptDialog from OCM endpoint
			disco, err := h.ocmClient.Discover(ctx, srv.URL)
			if err != nil {
				log.Warn().Err(err).
					Str("federation", fed.Federation).
					Str("server", srv.DisplayName).
					Str("url", srv.URL).
					Msg("Failed to discover server, skipping")
				discoveryErrors++
				continue
			}

			inviteDialog := disco.InviteAcceptDialog

			// If it's a relative path (starts with /), make it absolute
			if inviteDialog != "" && inviteDialog[0] == '/' {
				baseURL, parseErr := url.Parse(srv.URL)
				if parseErr == nil {
					inviteDialog = baseURL.Scheme + "://" + baseURL.Host + inviteDialog
					log.Debug().Str("original", disco.InviteAcceptDialog).Str("converted", inviteDialog).Msg("Converted relative path to absolute")
				} else {
					log.Warn().Err(parseErr).
						Str("url", srv.URL).
						Str("inviteDialog", disco.InviteAcceptDialog).
						Msg("Failed to parse server URL for relative path conversion")
					continue
				}
			}

			validServers = append(validServers, FederationServer{
				DisplayName:        srv.DisplayName,
				URL:                srv.URL,
				InviteAcceptDialog: inviteDialog,
			})
			validServersCount++

			log.Debug().
				Str("federation", fed.Federation).
				Str("server", srv.DisplayName).
				Str("inviteAcceptDialog", inviteDialog).
				Msg("Successfully discovered server")
		}

		if len(validServers) > 0 {
			h.federations = append(h.federations, Federation{
				Federation: fed.Federation,
				Servers:    validServers,
			})
			log.Debug().Str("federation", fed.Federation).Int("valid_servers", len(validServers)).Msg("Added federation with valid servers")
		} else {
			log.Warn().Str("federation", fed.Federation).
				Msg("Federation has no valid servers, skipping entirely")
		}
	}

	log.Info().
		Int("federations", len(h.federations)).
		Int("valid_servers", validServersCount).
		Int("discovery_errors", discoveryErrors).
		Msg("WAYF handler initialization completed")

	return nil
}

func (h *wayfHandler) GetFederations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(h.federations); err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error encoding response", err)
		return
	}
}

func (h *wayfHandler) DiscoverProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	var req DiscoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "Invalid request body", err)
		return
	}

	if req.Domain == "" {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "Domain is required", nil)
		return
	}

	domain := req.Domain
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}

	parsedURL, err := url.Parse(domain)
	if err != nil || parsedURL.Host == "" {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "Invalid domain format", err)
		return
	}

	log.Debug().Str("domain", domain).Msg("Attempting OCM discovery")
	disco, err := h.ocmClient.Discover(ctx, domain)
	if err != nil {
		log.Warn().Err(err).Str("domain", domain).Msg("Discovery failed")
		reqres.WriteError(w, r, reqres.APIErrorNotFound,
			fmt.Sprintf("Provider at '%s' does not support OCM discovery", req.Domain), err)
		return
	}

	inviteDialog := disco.InviteAcceptDialog
	if inviteDialog != "" && inviteDialog[0] == '/' {
		baseURL, _ := url.Parse(domain)
		inviteDialog = baseURL.Scheme + "://" + baseURL.Host + inviteDialog
	}

	response := DiscoverResponse{
		InviteAcceptDialog: inviteDialog,
	}

	log.Info().
		Str("domain", req.Domain).
		Str("inviteAcceptDialog", inviteDialog).
		Msg("Discovery successful")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "Error encoding response", err)
		return
	}
}
