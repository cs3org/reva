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

package apikey

import (
	"crypto/md5"
	"crypto/rand"
	"fmt"

	"github.com/pkg/errors"
)

// APIKey is the type used to store API keys.
type APIKey = string

const (
	// FlagDefault marks API keys for default (community) accounts.
	FlagDefault int16 = 0x0000
	// FlagScienceMesh marks API keys for ScienceMesh (partner) accounts.
	FlagScienceMesh int16 = 0x0001
)

const charset = "abcdefghijklmnopqrstuvwxyz" + "ABCDEFGHIJKLMNOPQRSTUVWXYZ" + "0123456789"

// GenerateAPIKey generates a new (random) API key which also contains flags and a (salted) hash.
// An API key has the following format:
//   <RandomString:30><Flags:2><SaltedHash:32>
func GenerateAPIKey(salt string, flags int16) (APIKey, error) {
	randomString, err := generateRandomString(30)
	if err != nil {
		return "", errors.Wrap(err, "unable to generate API key")
	}

	// To verify an API key, a hash is used which contains, beside the random string and flags, the email address
	hash := md5.New()
	hash.Write([]byte(randomString))
	hash.Write([]byte(salt))
	hash.Write([]byte(fmt.Sprintf("%04x", flags)))

	return fmt.Sprintf("%s%02x%032x", randomString, flags, hash.Sum(nil)), nil
}

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	str := ""
	for _, v := range b {
		str += string(charset[int(v)%len(charset)])
	}

	return str, nil
}
