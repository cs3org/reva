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

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// Capabilities is the subset of the OCS capabilities document the CLI acts on.
// Everything here changes behaviour: whether uploads go through TUS and at what
// chunk size, whether a recursive download can be served by the archiver in one
// request, which checksum algorithms the server will verify. Discovering these
// rather than hard-coding them is what lets one binary work against
// production, pre-production, and a developer's laptop.
type Capabilities struct {
	WebDavRoot string

	// TUS
	TusSupported    bool
	TusVersion      string
	TusMaxChunkSize int64

	// Checksums
	ChecksumTypes     []string
	PreferredChecksum string

	// Archiver, used for recursive downloads.
	ArchiverEnabled     bool
	ArchiverURL         string
	ArchiverFormats     []string
	ArchiverMaxNumFiles int64
	ArchiverMaxSize     int64

	// Feature flags.
	Versioning    bool
	Undelete      bool
	SpacesEnabled bool
	Sharing       bool
	PublicLinks   bool

	ServerVersion string
	ProductName   string
}

// ocsCapabilitiesResponse mirrors the OCS envelope. Only the fields the CLI
// reads are modelled; unknown fields are ignored, so a server that grows new
// capabilities does not break an older client.
type ocsCapabilitiesResponse struct {
	OCS struct {
		Meta struct {
			Status     string `json:"status"`
			StatusCode int    `json:"statuscode"`
			Message    string `json:"message"`
		} `json:"meta"`
		Data struct {
			Version struct {
				String  string `json:"string"`
				Edition string `json:"edition"`
			} `json:"version"`
			Capabilities struct {
				Core struct {
					WebdavRoot string `json:"webdav-root"`
					Status     struct {
						ProductName string `json:"productname"`
						Version     string `json:"versionstring"`
					} `json:"status"`
				} `json:"core"`
				Checksums struct {
					SupportedTypes      []string `json:"supportedTypes"`
					PreferredUploadType string   `json:"preferredUploadType"`
				} `json:"checksums"`
				Files struct {
					Undelete   ocsBool `json:"undelete"`
					Versioning ocsBool `json:"versioning"`
					TusSupport *struct {
						Version      string `json:"version"`
						Resumable    string `json:"resumable"`
						MaxChunkSize int64  `json:"max_chunk_size"`
					} `json:"tus_support"`
					Archivers []struct {
						Enabled     bool     `json:"enabled"`
						Formats     []string `json:"formats"`
						ArchiverURL string   `json:"archiver_url"`
						MaxNumFiles string   `json:"max_num_files"`
						MaxSize     string   `json:"max_size"`
					} `json:"archivers"`
				} `json:"files"`
				FilesSharing struct {
					APIEnabled ocsBool `json:"api_enabled"`
					Public     struct {
						Enabled ocsBool `json:"enabled"`
					} `json:"public"`
				} `json:"files_sharing"`
				Spaces *struct {
					Enabled ocsBool `json:"enabled"`
				} `json:"spaces"`
			} `json:"capabilities"`
		} `json:"data"`
	} `json:"ocs"`
}

// ocsBool decodes the several shapes OCS uses for a boolean. The server emits
// true, "1", or 1 depending on the field and the serialiser, so a plain bool
// would fail to decode on fields that happen to use the string form.
type ocsBool bool

func (b *ocsBool) UnmarshalJSON(data []byte) error {
	s := string(data)
	switch s {
	case "true", `"1"`, "1", `"true"`:
		*b = true
	case "false", `"0"`, "0", `"false"`, "null", `""`:
		*b = false
	default:
		var v bool
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*b = ocsBool(v)
	}
	return nil
}

// Capabilities fetches and caches the server's capabilities. It is fetched at
// most once per process.
func (c *Client) Capabilities(ctx context.Context) (*Capabilities, error) {
	c.capsOnce.Do(func() {
		c.caps, c.capsErr = c.fetchCapabilities(ctx)
	})
	return c.caps, c.capsErr
}

func (c *Client) fetchCapabilities(ctx context.Context) (*Capabilities, error) {
	req := request{
		method: http.MethodGet,
		url:    c.URL(ocsCapabilities) + "?format=json",
		header: http.Header{"OCS-APIREQUEST": []string{"true"}},
		op:     "read server capabilities",
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw ocsCapabilitiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, cberr.Wrap(cberr.KindOther, "read server capabilities", "", err)
	}

	d := raw.OCS.Data
	caps := &Capabilities{
		WebDavRoot:        d.Capabilities.Core.WebdavRoot,
		ChecksumTypes:     d.Capabilities.Checksums.SupportedTypes,
		PreferredChecksum: d.Capabilities.Checksums.PreferredUploadType,
		Versioning:        bool(d.Capabilities.Files.Versioning),
		Undelete:          bool(d.Capabilities.Files.Undelete),
		Sharing:           bool(d.Capabilities.FilesSharing.APIEnabled),
		PublicLinks:       bool(d.Capabilities.FilesSharing.Public.Enabled),
		ServerVersion:     d.Version.String,
		ProductName:       d.Capabilities.Core.Status.ProductName,
	}
	if d.Capabilities.Spaces != nil {
		caps.SpacesEnabled = bool(d.Capabilities.Spaces.Enabled)
	}
	if t := d.Capabilities.Files.TusSupport; t != nil && t.Version != "" {
		caps.TusSupported = true
		caps.TusVersion = t.Version
		caps.TusMaxChunkSize = t.MaxChunkSize
	}
	for _, a := range d.Capabilities.Files.Archivers {
		if !a.Enabled || a.ArchiverURL == "" {
			continue
		}
		caps.ArchiverEnabled = true
		caps.ArchiverURL = a.ArchiverURL
		caps.ArchiverFormats = a.Formats
		// These arrive as strings because the OCS schema declares them so.
		caps.ArchiverMaxNumFiles, _ = strconv.ParseInt(a.MaxNumFiles, 10, 64)
		caps.ArchiverMaxSize, _ = strconv.ParseInt(a.MaxSize, 10, 64)
		break
	}
	return caps, nil
}

// SupportsArchiveFormat reports whether the server's archiver can produce the
// named format.
func (c *Capabilities) SupportsArchiveFormat(format string) bool {
	if c == nil || !c.ArchiverEnabled {
		return false
	}
	return slices.Contains(c.ArchiverFormats, format)
}

// ChunkSize returns the upload chunk size to use, clamped to what the server
// advertises. A server that advertises no limit gets the caller's preference.
func (c *Capabilities) ChunkSize(preferred int64) int64 {
	if c == nil || c.TusMaxChunkSize <= 0 {
		return preferred
	}
	if preferred <= 0 || preferred > c.TusMaxChunkSize {
		return c.TusMaxChunkSize
	}
	return preferred
}

// BestChecksum picks a checksum algorithm the server will verify, preferring
// the one it names as preferred and falling back through the supported list in
// descending order of strength.
func (c *Capabilities) BestChecksum() string {
	if c == nil {
		return ""
	}
	if c.PreferredChecksum != "" {
		return c.PreferredChecksum
	}
	for _, want := range []string{"sha1", "md5", "adler32"} {
		if slices.Contains(c.ChecksumTypes, want) {
			return want
		}
	}
	return ""
}
