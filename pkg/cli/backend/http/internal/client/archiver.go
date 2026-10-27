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
	"net/url"
	"slices"
	"strings"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// ArchiveURL returns the URL of the archiver for paths in format.
func (c *Client) ArchiveURL(ctx context.Context, paths []string, format string) (string, error) {
	caps, err := c.Capabilities(ctx)
	if err != nil {
		return "", err
	}
	if !caps.ArchiverEnabled {
		return "", cberr.New(cberr.KindOther, "download archive", strings.Join(paths, ", "),
			"this server has no archiver service")
	}
	if !slices.Contains(caps.ArchiverFormats, format) {
		return "", cberr.Usagef("this server cannot produce %q archives, it supports: %s",
			format, strings.Join(caps.ArchiverFormats, ", "))
	}
	if len(paths) == 0 {
		return "", cberr.Usagef("no paths to archive")
	}

	// The archiver URL can have a query already. Parse it, so that we keep
	// its parameters.
	ref, err := url.Parse(caps.ArchiverURL)
	if err != nil {
		return "", cberr.Wrap(cberr.KindOther, "download archive", "", err)
	}
	q := ref.Query()
	for _, p := range paths {
		q.Add("path", p)
	}
	q.Set("arch_type", format)
	ref.RawQuery = q.Encode()

	u := c.URL(ref.EscapedPath())
	if ref.RawQuery != "" {
		u += "?" + ref.RawQuery
	}
	return u, nil
}
