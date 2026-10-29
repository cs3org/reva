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

package client

import "context"

// Credential returns the credential that the requests of the client carry.
func (c *Client) Credential(ctx context.Context) (Credential, error) {
	if c.creds == nil {
		return Credential{}, nil
	}
	return c.creds.Credential(ctx)
}

// DavURL returns the WebDAV URL of the absolute path p.
func (c *Client) DavURL(ctx context.Context, p string) (string, error) { return c.davURL(ctx, p) }

// VersionURL returns the URL of the version key of the resource id.
func (c *Client) VersionURL(resourceID, key string) string { return c.versionsURL(resourceID, key) }
