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

package credentials

// Credentials stores and en-/decrypts credentials
type Credentials struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

// Get decrypts and retrieves the stored credentials.
func (creds *Credentials) Get() (string, string) {
	// TODO: Decrypt
	id := creds.ID
	secret := creds.Secret

	return id, secret
}

// Set encrypts and sets new credentials.
func (creds *Credentials) Set(id, secret string) error {
	// TODO: Encrypt
	creds.ID = id
	creds.Secret = secret

	return nil
}

// IsValid checks whether the credentials are valid.
func (creds *Credentials) IsValid() bool {
	return len(creds.ID) > 0 && len(creds.Secret) > 0
}

// Clear resets the credentials.
func (creds *Credentials) Clear() {
	creds.ID = ""
	creds.Secret = ""
}
