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

// Capabilities are the server abilities that the CLI uses.
type Capabilities struct {
	TusSupported    bool
	TusMaxChunkSize int64

	ChecksumTypes     []string
	PreferredChecksum string

	ArchiverEnabled     bool
	ArchiverURL         string
	ArchiverFormats     []string
	ArchiverMaxNumFiles int64
	ArchiverMaxSize     int64

	Versioning    bool
	ServerVersion string
	ProductName   string
}

// ocsCapabilitiesResponse has only the fields that the CLI reads. A new
// field on the server does not break an old client.
type ocsCapabilitiesResponse struct {
	OCS struct {
		Data struct {
			Version struct {
				String string `json:"string"`
			} `json:"version"`
			Capabilities struct {
				Core struct {
					Status struct {
						ProductName string `json:"productname"`
					} `json:"status"`
				} `json:"core"`
				Checksums struct {
					SupportedTypes      []string `json:"supportedTypes"`
					PreferredUploadType string   `json:"preferredUploadType"`
				} `json:"checksums"`
				Files struct {
					Versioning ocsBool `json:"versioning"`
					TusSupport *struct {
						Version      string `json:"version"`
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
			} `json:"capabilities"`
		} `json:"data"`
	} `json:"ocs"`
}

// ocsBool decodes true, "1" and 1, because the server uses all of them.
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

// Capabilities fetches the capabilities of the server once, and keeps them.
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
		ChecksumTypes:     d.Capabilities.Checksums.SupportedTypes,
		PreferredChecksum: d.Capabilities.Checksums.PreferredUploadType,
		Versioning:        bool(d.Capabilities.Files.Versioning),
		ServerVersion:     d.Version.String,
		ProductName:       d.Capabilities.Core.Status.ProductName,
	}
	if t := d.Capabilities.Files.TusSupport; t != nil && t.Version != "" {
		caps.TusSupported = true
		caps.TusMaxChunkSize = t.MaxChunkSize
	}
	for _, a := range d.Capabilities.Files.Archivers {
		if !a.Enabled || a.ArchiverURL == "" {
			continue
		}
		caps.ArchiverEnabled = true
		caps.ArchiverURL = a.ArchiverURL
		caps.ArchiverFormats = a.Formats
		// The OCS schema declares these two as strings.
		caps.ArchiverMaxNumFiles, _ = strconv.ParseInt(a.MaxNumFiles, 10, 64)
		caps.ArchiverMaxSize, _ = strconv.ParseInt(a.MaxSize, 10, 64)
		break
	}
	return caps, nil
}

// BestChecksum returns the checksum type that the server prefers, or else the
// strongest one that it supports.
func (c *Capabilities) BestChecksum() string {
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
