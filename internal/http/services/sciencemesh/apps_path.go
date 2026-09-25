// Copyright 2018-2024 CERN
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
	"errors"
	"net/url"
	"strings"

	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func validateShareFilePath(filePath string) error {
	unescaped, err := fullyUnescape(filePath)
	if err != nil {
		return errtypes.BadRequest("malformed file path")
	}
	if strings.Contains(unescaped, `\`) || strings.Contains(unescaped, "://") {
		return errtypes.BadRequest("invalid file path")
	}
	for _, seg := range strings.Split(unescaped, "/") {
		if seg == ".." {
			return errtypes.BadRequest("file path escapes the share")
		}
	}
	return nil
}

// joinShareRelativePath appends a share-relative file path. An empty relative
// path keeps the bare opener unchanged. Share identity is not added here.
func joinShareRelativePath(appURI, rel string) (string, error) {
	if rel == "" {
		return appURI, nil
	}
	segments, err := relativePathSegments(rel)
	if err != nil {
		return "", err
	}
	escaped := make([]string, len(segments))
	for i, seg := range segments {
		escaped[i] = url.PathEscape(seg)
	}
	joined, err := url.JoinPath(appURI, escaped...)
	if err != nil {
		return "", errtypes.BadRequest("invalid share-relative path")
	}
	parsed, err := url.Parse(joined)
	if err != nil {
		return "", errtypes.BadRequest("invalid share-relative path")
	}
	if err := validateLaunchURL(parsed); err != nil {
		return "", err
	}
	suffix := "/" + strings.Join(escaped, "/")
	if !strings.HasSuffix(parsed.EscapedPath(), suffix) {
		return "", errtypes.BadRequest("invalid share-relative path")
	}
	return joined, nil
}

func relativePathSegments(rel string) ([]string, error) {
	unescaped, err := fullyUnescape(rel)
	if err != nil {
		return nil, errtypes.BadRequest("malformed share-relative path")
	}
	absolutePath := strings.HasPrefix(unescaped, "/")
	hasBackslash := strings.Contains(unescaped, `\`)
	hasScheme := strings.Contains(unescaped, "://")
	if unescaped == "" || absolutePath || hasBackslash || hasScheme {
		return nil, errtypes.BadRequest("invalid share-relative path")
	}
	parsed, err := url.Parse(unescaped)
	if err != nil || parsed.IsAbs() {
		return nil, errtypes.BadRequest("invalid share-relative path")
	}
	parts := strings.Split(unescaped, "/")
	segments := make([]string, 0, len(parts))
	for _, seg := range parts {
		if seg == "" || seg == "." || seg == ".." {
			return nil, errtypes.BadRequest("invalid share-relative path")
		}
		segments = append(segments, seg)
	}
	return segments, nil
}

func fullyUnescape(raw string) (string, error) {
	current := raw
	for range 8 {
		next, err := url.PathUnescape(current)
		if err != nil {
			return "", err
		}
		if next == current {
			return current, nil
		}
		current = next
	}
	return "", errors.New("too many escape layers")
}
