// Copyright 2018-2026 CERN
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

package ocmd

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/ocm/httpsig"
)

// verifySignature checks the OCM signature of r against the keys of the remote
// server that disco describes. endpoint is the public URL of this server.
func verifySignature(ctx context.Context, c *OCMClient, r *http.Request, body []byte, endpoint string, disco *wellknown.OcmDiscoveryData) error {
	err := httpsig.Verify(r, body, endpoint, func() ([]byte, error) {
		if disco.JwksURI == "" {
			return nil, errors.New("the remote OCM server publishes no jwksUri")
		}
		return c.httpget(ctx, disco.JwksURI, DefaultResponseLimit)
	})
	// A server that signs its requests must not be accepted without a signature,
	// else an attacker can send unsigned requests in its name.
	if errors.Is(err, httpsig.ErrUnsigned) && !slices.Contains(disco.Capabilities, "http-sig") {
		return nil
	}
	return err
}
