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

package password

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/cs3org/reva/pkg/utils"
	"github.com/pkg/errors"
)

// Password holds a hash password alongside its salt value.
type Password struct {
	Value string `json:"value"`
	Salt  string `json:"salt"`
}

const (
	passwordMinLength  = 8
	passwordSaltLength = 16
)

// IsValid checks whether the password is valid.
func (password *Password) IsValid() bool {
	return len(password.Value) == 64 && len(password.Salt) == passwordSaltLength
}

// Clear resets the password.
func (password *Password) Clear() {
	password.Value = ""
	password.Salt = ""
}

// Compare checks whether the given password string equals the stored one.
func (password *Password) Compare(pwd string) bool {
	hashedPwd := hashPassword(pwd, password.Salt)
	return hashedPwd == password.Value
}

// GeneratePassword salts and hashes the given password.
func GeneratePassword(pwd string) (*Password, error) {
	if err := VerifyPassword(pwd); err != nil {
		return nil, errors.Wrap(err, "invalid password")
	}

	// Create a random salt string
	salt := utils.RandString(passwordSaltLength)

	return &Password{Value: hashPassword(pwd, salt), Salt: salt}, nil
}

func hashPassword(pwd, salt string) string {
	saltedPwd := pwd + salt

	// Value the salted password using SHA256
	h := sha256.New()
	h.Write([]byte(saltedPwd))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// VerifyPassword checks whether the given password abides to the enforced password strength.
func VerifyPassword(pwd string) error {
	if len(pwd) < passwordMinLength {
		return errors.Errorf("the password must be at least 8 characters long")
	}
	if !strings.ContainsAny(pwd, "abcdefghijklmnopqrstuvwxyz") {
		return errors.Errorf("the password must contain at least one lowercase letter")
	}
	if !strings.ContainsAny(pwd, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return errors.Errorf("the password must contain at least one uppercase letter")
	}
	if !strings.ContainsAny(pwd, "0123456789") {
		return errors.Errorf("the password must contain at least one digit")
	}

	return nil
}
