// Copyright 2018-2023 CERN
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

package crypto

import (
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"hash"
	"hash/adler32"
	"io"
)

const bufferSize = 4096

// computeHashXS computes the hash checksum for a given hash.Hash implementation.
func computeHashXS(r io.Reader, h hash.Hash) (string, error) {
	buf := make([]byte, bufferSize)

	for {
		n, err := r.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}

		if n > 0 {
			h.Write(buf[:n])
		}

		if err == io.EOF {
			break
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// ComputeMD5XS computes the MD5 checksum.
func ComputeMD5XS(r io.Reader) (string, error) {
	return computeHashXS(r, md5.New())
}

// ComputeAdler32XS computes the adler32 checksum.
func ComputeAdler32XS(r io.Reader) (string, error) {
	return computeHashXS(r, adler32.New())
}

// ComputeSHA1XS computes the sha1 checksum.
func ComputeSHA1XS(r io.Reader) (string, error) {
	return computeHashXS(r, sha1.New())
}
