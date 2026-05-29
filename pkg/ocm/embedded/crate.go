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

package embedded

import (
	"encoding/json"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
)

type crate struct {
	Graph []crateEntity `json:"@graph"`
}

type crateEntity struct {
	ID             string               `json:"@id"`
	Type           json.RawMessage      `json:"@type"`
	URL            json.RawMessage      `json:"url"`
	Name           string               `json:"name"`
	ContentSize    string               `json:"contentSize"`
	EncodingFormat string               `json:"encodingFormat"`
	Description    string               `json:"description"`
	Distribution   []zenodoDistribution `json:"distribution"`
}

type zenodoDistribution struct {
	Type           string `json:"@type"`
	ContentURL     string `json:"contentUrl"`
	EncodingFormat string `json:"encodingFormat"`
}

type transferEntry struct {
	srcURL         string
	name           string
	sizeHint       int64
	encodingFormat string
}

type idRef struct {
	ID string `json:"@id"`
}

func (e crateEntity) URLString() string {
	if len(e.URL) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(e.URL, &s); err == nil {
		return s
	}

	var ref idRef
	if err := json.Unmarshal(e.URL, &ref); err == nil {
		return ref.ID
	}

	return ""
}

func (e crateEntity) HasType(want string) bool {
	if len(e.Type) == 0 {
		return false
	}

	var single string
	if err := json.Unmarshal(e.Type, &single); err == nil {
		return single == want
	}

	var many []string
	if err := json.Unmarshal(e.Type, &many); err == nil {
		for _, t := range many {
			if t == want {
				return true
			}
		}
	}

	return false
}

func (e crateEntity) IsTransferable() bool {
	if e.URLString() == "" {
		return false
	}

	return e.HasType("File") || e.HasType("ComputationalWorkflow") || e.HasType("SoftwareSourceCode")
}

// crateEntries collects every transferable file from the RO-Crate @graph: both
// plain entities (a File with a direct url) and Zenodo Datasets (whose
// distribution[] holds DataDownload entries with a contentUrl).
func crateEntries(log *zerolog.Logger, graph []crateEntity) []transferEntry {
	var entries []transferEntry
	for _, e := range graph {
		if e.IsTransferable() {
			if entry, ok := plainEntry(log, e); ok {
				entries = append(entries, entry)
			}
		}
		for _, d := range e.Distribution {
			if entry, ok := zenodoEntry(log, d); ok {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func plainEntry(log *zerolog.Logger, e crateEntity) (transferEntry, bool) {
	srcURL := e.URLString()
	if srcURL == "" {
		return transferEntry{}, false
	}

	name := strings.TrimSpace(e.Name)
	if name == "" {
		name = path.Base(srcURL)
	}
	if name == "" || name == "." || name == "/" {
		log.Warn().
			Str("entity_id", e.ID).
			Str("src", srcURL).
			Msg("Skipping entity with unusable destination name")
		return transferEntry{}, false
	}

	size := int64(-1)
	if e.ContentSize != "" {
		if parsed, err := strconv.ParseInt(e.ContentSize, 10, 64); err == nil {
			size = parsed
		}
	}

	return transferEntry{
		srcURL:         srcURL,
		name:           name,
		sizeHint:       size,
		encodingFormat: e.EncodingFormat,
	}, true
}

func zenodoEntry(log *zerolog.Logger, d zenodoDistribution) (transferEntry, bool) {
	if d.Type != "DataDownload" || d.ContentURL == "" {
		return transferEntry{}, false
	}

	name := zenodoFilename(d.ContentURL)
	if name == "" || name == "." || name == "/" {
		log.Warn().
			Str("src", d.ContentURL).
			Msg("Skipping Zenodo distribution with unusable destination name")
		return transferEntry{}, false
	}

	return transferEntry{
		srcURL:         d.ContentURL,
		name:           name,
		sizeHint:       -1,
		encodingFormat: d.EncodingFormat,
	}, true
}

func zenodoFilename(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Path == "" {
		return path.Base(rawURL)
	}
	return path.Base(strings.TrimSuffix(u.Path, "/content"))
}
