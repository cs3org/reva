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

package key

import (
	"fmt"
	"hash/crc64"

	"github.com/pkg/errors"
)

// SiteIdentifier is the type used to store site identifiers.
type SiteIdentifier = string

// CalculateSiteID calculates a (stable) site ID from the given API key.
// The site ID is actually the CRC64 hash of the provided API key plus a salt value, thus it is stable for any given key & salt pair.
func CalculateSiteID(apiKey APIKey, salt string) (SiteIdentifier, error) {
	if len(apiKey) != apiKeyLength {
		return "", errors.Errorf("invalid API key length")
	}

	hash := crc64.New(crc64.MakeTable(crc64.ECMA))
	_, _ = hash.Write([]byte(apiKey))
	_, _ = hash.Write([]byte(salt))
	value := hash.Sum(nil)
	return fmt.Sprintf("%4x-%4x-%4x-%4x", value[:2], value[2:4], value[4:6], value[6:]), nil
}
