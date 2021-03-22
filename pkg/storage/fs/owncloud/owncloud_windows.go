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

// +build windows

package owncloud

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"github.com/cs3org/reva/pkg/appctx"
)

// calcEtag will create an etag based on the md5 of
// - mtime,
// - inode (if available),
// - device (if available) and
// - size.
// errors are logged, but an etag will still be returned
func calcEtag(ctx context.Context, fi os.FileInfo) string {
	log := appctx.GetLogger(ctx)
	h := md5.New()
	err := binary.Write(h, binary.BigEndian, fi.ModTime().UnixNano())
	if err != nil {
		log.Error().Err(err).Msg("error writing mtime")
	}
	// device and inode have no meaning on windows
	err = binary.Write(h, binary.BigEndian, fi.Size())
	if err != nil {
		log.Error().Err(err).Msg("error writing size")
	}
	etag := fmt.Sprintf(`"%x"`, h.Sum(nil))
	return fmt.Sprintf("\"%s\"", strings.Trim(etag, "\""))
}
