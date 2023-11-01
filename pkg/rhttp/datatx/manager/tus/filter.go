// Copyright 2018-2021 CERN
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

package tus

import (
	"net/http"
	"strings"

	tusd "github.com/tus/tusd/pkg/handler"
)

type FilterResponseWriter struct {
	w      http.ResponseWriter
	header http.Header
}

const TusPrefix = "tus."
const CS3Prefix = "cs3."

// NewFilterResponseWriter wraps the given http.ResponseWriter and filters out any metadata prefixed with
// 'cs3.'. Metadata prefixed with 'tus.' will be returned without the prefix.
func NewFilterResponseWriter(w http.ResponseWriter) *FilterResponseWriter {
	return &FilterResponseWriter{
		w:      w,
		header: http.Header{},
	}
}

func (fw *FilterResponseWriter) Header() http.Header {
	return fw.w.Header()
}

func (fw *FilterResponseWriter) Write(b []byte) (int, error) {
	return fw.w.Write(b)
}

func (fw *FilterResponseWriter) WriteHeader(statusCode int) {
	metadata := tusd.ParseMetadataHeader(fw.w.Header().Get("Upload-Metadata"))
	tusMetadata := map[string]string{}
	for k, v := range metadata {
		if strings.HasPrefix(k, TusPrefix) {
			tusMetadata[strings.TrimPrefix(k, TusPrefix)] = v
		}
	}

	fw.w.Header().Set("Upload-Metadata", tusd.SerializeMetadataHeader(tusMetadata))
	fw.w.WriteHeader(statusCode)
}
