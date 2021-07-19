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

package manager

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cs3org/reva/pkg/siteacc/html"
	"github.com/pkg/errors"
	"github.com/sethvargo/go-password/password"
)

type userToken struct {
	SessionID string
	User      string
}

const (
	tokenKeyLength = 16

	tokenNonceName = "usertoken_nonce"
)

var (
	tokenKey string
)

func generateUserToken(session *html.Session) (string, error) {
	// The token consists of the session ID and the logged in user's email address
	token := userToken{
		SessionID: session.ID,
		User:      session.LoggedInUser.Email,
	}

	data, err := json.Marshal(&token)
	if err != nil {
		return "", errors.Wrap(err, "unable to marshal the token")
	}

	// Encrypt the data using AES
	block, _ := aes.NewCipher([]byte(tokenKey))
	aesgcm, _ := cipher.NewGCM(block)

	// Generate a nonce and store it in the session
	nonce := make([]byte, aesgcm.NonceSize())
	_, _ = io.ReadFull(rand.Reader, nonce)
	session.Data[tokenNonceName] = nonce

	cipherText := fmt.Sprintf("%x", aesgcm.Seal(nil, nonce, data, nil))

	return cipherText, nil
}

func extractUserToken(token string, session *html.Session) (*userToken, error) {
	// Get the nonce from the session
	var nonce []byte
	if nonceData, ok := session.Data[tokenNonceName]; ok {
		nonce, ok = nonceData.([]byte)
		if !ok {
			return nil, errors.Errorf("invalid nonce stored in the current session")
		}
	} else {
		return nil, errors.Errorf("no nonce found in the current session")
	}

	// Decrypt the data using AES
	cipherText, _ := hex.DecodeString(token)

	block, _ := aes.NewCipher([]byte(tokenKey))
	aesgcm, _ := cipher.NewGCM(block)

	plainText, err := aesgcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return nil, errors.Wrap(err, "unable to decrypt the token")
	}

	var utoken userToken
	if err := json.Unmarshal(plainText, &utoken); err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal the token")
	}
	return &utoken, nil
}

func init() {
	// Generate the key used for AES encryption
	tokenKey = password.MustGenerate(tokenKeyLength, tokenKeyLength/4, tokenKeyLength/4, false, true)
}
