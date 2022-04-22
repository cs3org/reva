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

package mock

import (
	"bufio"
	"context"
	"io"
	"os"

	"github.com/cs3org/reva/v2/pkg/storage/utils/downloader"

	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

type mockDownloader struct{}

// NewDownloader creates a mock downloader that implements the Downloader interface
// supposed to be used for testing
func NewDownloader() downloader.Downloader {
	return &mockDownloader{}
}

// Download copies the content of a local file into the dst Writer
func (m *mockDownloader) Download(ctx context.Context, id *providerv1beta1.ResourceId, dst io.Writer) error {
	f, err := os.Open(id.OpaqueId)
	if err != nil {
		return err
	}
	defer f.Close()
	fr := bufio.NewReader(f)
	_, err = io.Copy(dst, fr)
	return err
}
