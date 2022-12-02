// Copyright 2018-2022 CERN
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

package mock

import (
	"context"
	"io"
	"os"

	"github.com/cs3org/reva/pkg/storage/utils/downloader"
)

type mockDownloader struct{}

// NewDownloader creates a mock downloader that implements the Downloader interface
// supposed to be used for testing.
func NewDownloader() downloader.Downloader {
	return &mockDownloader{}
}

// Download copies the content of a local file into the dst Writer
func (m *mockDownloader) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	return os.Open(path)
}
