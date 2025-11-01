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
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"
)

type wayfHandler struct {
	directoryServices []ocmd.DirectoryService
	ocmClient         *ocmd.OCMClient
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
	h.ocmClient = ocmd.NewClientWithConfig(&c.OCMClient)
	log.Debug().Msg("Created OCM client for discovery")

	urls := strings.Fields(c.DirectoryServiceURLs)
	if len(urls) == 0 {
		log.Info().Msg("No directory service URLs configured, starting with empty list")
		h.directoryServices = []ocmd.DirectoryService{}
		return nil
	}

	log.Debug().Int("url_count", len(urls)).Strs("urls", urls).Msg("Initializing WAYF handler with directory service URLs")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	h.directoryServices = []ocmd.DirectoryService{}
	discoveryErrors := 0
	validServersCount := 0
	fetchErrors := 0

	for _, directoryURL := range urls {
		log.Debug().Str("url", directoryURL).Msg("Fetching directory service")

		directoryService, err := h.ocmClient.GetDirectoryService(ctx, directoryURL)
		if err != nil {
			log.Info().Err(err).Str("url", directoryURL).Msg("Failed to fetch directory service, skipping")
			fetchErrors++
			continue
		}

		log.Debug().Str("federation", directoryService.Federation).Int("servers_count", len(directoryService.Servers)).Msg("Processing directory service")

		var validServers []ocmd.DirectoryServiceServer
		for _, srv := range directoryService.Servers {
			if srv.DisplayName == "" || srv.URL == "" {
				log.Debug().Str("federation", directoryService.Federation).
					Str("displayName", srv.DisplayName).
					Str("url", srv.URL).
					Msg("Skipping server with missing displayName or url")
				continue
			}

			log.Debug().Str("federation", directoryService.Federation).Str("server", srv.DisplayName).Str("url", srv.URL).Msg("Discovering server")

			// Discover inviteAcceptDialog from OCM endpoint
			disco, err := h.ocmClient.Discover(ctx, srv.URL)
			if err != nil {
				log.Debug().Err(err).
					Str("federation", directoryService.Federation).
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
					log.Debug().Err(parseErr).
						Str("url", srv.URL).
						Str("inviteDialog", disco.InviteAcceptDialog).
						Msg("Failed to parse server URL for relative path conversion")
					continue
				}
			}

			validServers = append(validServers, ocmd.DirectoryServiceServer{
				DisplayName:        srv.DisplayName,
				URL:                srv.URL,
				InviteAcceptDialog: inviteDialog,
			})
			validServersCount++

			log.Debug().
				Str("federation", directoryService.Federation).
				Str("server", srv.DisplayName).
				Str("inviteAcceptDialog", inviteDialog).
				Msg("Successfully discovered server")
		}

		if len(validServers) > 0 {
			h.directoryServices = append(h.directoryServices, ocmd.DirectoryService{
				Federation: directoryService.Federation,
				Servers:    validServers,
			})
			log.Debug().Str("federation", directoryService.Federation).Int("valid_servers", len(validServers)).Msg("Added directory service with valid servers")
		} else {
			log.Info().Str("federation", directoryService.Federation).
				Msg("Directory service has no valid servers, skipping entirely")
		}
	}

	log.Info().
		Int("directory_services", len(h.directoryServices)).
		Int("valid_servers", validServersCount).
		Int("fetch_errors", fetchErrors).
		Int("discovery_errors", discoveryErrors).
		Msg("WAYF handler initialization completed")

	return nil
}

func (h *wayfHandler) GetFederations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(h.directoryServices); err != nil {
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
		log.Info().Err(err).Str("domain", domain).Msg("Discovery failed")
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
