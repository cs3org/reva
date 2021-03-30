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
	"crypto/md5"
	"crypto/rand"
	"fmt"
	hashpkg "hash"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// APIKey is the type used to store API keys.
type APIKey = string

const (
	// FlagDefault marks API keys for default (community) accounts.
	FlagDefault = 0x0000
	// FlagScienceMesh marks API keys for ScienceMesh (partner) accounts.
	FlagScienceMesh = 0x0001
)

const (
	randomStringLength = 30
	apiKeyLength       = randomStringLength + 2 + 32

	charset = "abcdefghijklmnopqrstuvwxyz" + "ABCDEFGHIJKLMNOPQRSTUVWXYZ" + "0123456789"
)

// GenerateAPIKey generates a new (random) API key which also contains flags and a (salted) hash.
// An API key has the following format:
//   <RandomString:30><Flags:2><SaltedHash:32>
func GenerateAPIKey(salt string, flags int) (APIKey, error) {
	if len(salt) == 0 {
		return "", errors.Errorf("no salt specified")
	}

	randomString, err := generateRandomString(randomStringLength)
	if err != nil {
		return "", errors.Wrap(err, "unable to generate API key")
	}

	// To verify an API key, a hash is used which contains, beside the random string and flags, the email address
	hash := calculateHash(randomString, flags, salt)
	return fmt.Sprintf("%s%02x%032x", randomString, flags, hash.Sum(nil)), nil
}

// VerifyAPIKey checks if the API key is valid given the specified salt value.
func VerifyAPIKey(apiKey APIKey, salt string) error {
	randomString, flags, hash, err := SplitAPIKey(apiKey)
	if err != nil {
		return errors.Wrap(err, "error while extracting API key information")
	}

	hashCalc := calculateHash(randomString, flags, salt)
	if fmt.Sprintf("%032x", hashCalc.Sum(nil)) != hash {
		return errors.Errorf("the API key is invalid")
	}

	return nil
}

// SplitAPIKey splits an API key into its pieces: RandomString, Flags and Hash.
func SplitAPIKey(apiKey APIKey) (string, int, string, error) {
	if len(apiKey) != apiKeyLength {
		return "", 0, "", errors.Errorf("invalid API key length")
	}

	randomString := apiKey[:randomStringLength]
	flags, err := strconv.Atoi(apiKey[randomStringLength : randomStringLength+2])
	if err != nil {
		return "", 0, "", errors.Errorf("invalid API key format")
	}
	hash := apiKey[randomStringLength+2:]

	return randomString, flags, hash, nil
}

// SaltFromEmail generates a salt-value from an email address.
func SaltFromEmail(email string) string {
	return strings.ToLower(email)
}

func calculateHash(randomString string, flags int, salt string) hashpkg.Hash {
	hash := md5.New()
	_, _ = hash.Write([]byte(randomString))
	_, _ = hash.Write([]byte(salt))
	_, _ = hash.Write([]byte(fmt.Sprintf("%04x", flags)))
	return hash
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
