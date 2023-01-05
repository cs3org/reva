// Copyright 2018-2023 CERN
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

package capabilities

import (
	"context"
	"strings"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/data"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/juliangruber/go-intersect"
)

type chunkProtocol string

var (
	chunkV1  chunkProtocol = "v1"
	chunkNG  chunkProtocol = "ng"
	chunkTUS chunkProtocol = "tus"
)

func (h *Handler) getCapabilitiesForUserAgent(ctx context.Context, userAgent string) data.CapabilitiesData {
	// Creating a copy of the capabilities struct is less expensive than taking a lock
	c := *h.c.Capabilities
	if userAgent != "" {
		for k, v := range h.userAgentChunkingMap {
			// we could also use a regexp for pattern matching
			if strings.Contains(userAgent, k) {
				setCapabilitiesForChunkProtocol(chunkProtocol(v), &c)
			}
		}
	}

	c.GroupBased.Capabilities = []string{}
	for capability, groups := range h.groupBasedCapabilities {
		if ctxUserBelongsToGroups(ctx, groups) {
			c.GroupBased.Capabilities = append(c.GroupBased.Capabilities, capability)
		}
	}

	return data.CapabilitiesData{Capabilities: &c, Version: h.c.Version}
}

func ctxUserBelongsToGroups(ctx context.Context, groups []string) bool {
	if user, ok := ctxpkg.ContextGetUser(ctx); ok {
		return len(intersect.Simple(groups, user.Groups)) > 0
	}
	return false
}

func setCapabilitiesForChunkProtocol(cp chunkProtocol, c *data.Capabilities) {
	switch cp {
	case chunkV1:
		// 2.7+ will use Chunking V1 if "capabilities > files > bigfilechunking" is "true" AND "capabilities > dav > chunking" is not there
		c.Files.BigFileChunking = true
		c.Dav = nil
		c.Files.TusSupport = nil

	case chunkNG:
		// 2.7+ will use Chunking NG if "capabilities > files > bigfilechunking" is "true" AND "capabilities > dav > chunking" = 1.0
		c.Files.BigFileChunking = true
		c.Dav.Chunking = "1.0"
		c.Files.TusSupport = nil

	case chunkTUS:
		// 2.7+ will use TUS if "capabilities > files > bigfilechunking" is "false" AND "capabilities > dav > chunking" = "" AND "capabilities > files > tus_support" has proper entries.
		c.Files.BigFileChunking = false
		c.Dav.Chunking = ""

		// TODO: infer from various TUS handlers from all known storages
		// until now we take the manually configured tus options
		// c.Capabilities.Files.TusSupport = &data.CapabilitiesFilesTusSupport{
		// 	Version:            "1.0.0",
		// 	Resumable:          "1.0.0",
		// 	Extension:          "creation,creation-with-upload",
		// 	MaxChunkSize:       0,
		// 	HTTPMethodOverride: "",
		// }
	}
}
