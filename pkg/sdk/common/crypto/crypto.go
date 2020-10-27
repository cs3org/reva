// Copyright 2018-2020 CERN
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
	"fmt"
	"io"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// ComputeChecksum calculates the checksum of the given data using the specified checksum type.
func ComputeChecksum(checksumType provider.ResourceChecksumType, data io.Reader) (string, error) {
	switch checksumType {
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32:
		return ComputeAdler32Checksum(data)
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5:
		return ComputeMD5Checksum(data)
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_SHA1:
		return ComputeSHA1Checksum(data)
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET:
		return "", nil
	default:
		return "", fmt.Errorf("invalid checksum type: %s", checksumType)
	}
}

// GetChecksumTypeName returns a stringified name of the given checksum type.
func GetChecksumTypeName(checksumType provider.ResourceChecksumType) string {
	switch checksumType {
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET:
		return "unset"
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_SHA1:
		return "sha1"
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32:
		return "adler32"
	case provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5:
		return "md5"
	default:
		return "invalid"
	}
}
