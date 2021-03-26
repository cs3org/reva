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

package publicshare

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"time"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

// Manager manipulates public shares.
type Manager interface {
	CreatePublicShare(ctx context.Context, u *user.User, md *provider.ResourceInfo, g *link.Grant) (*link.PublicShare, error)
	UpdatePublicShare(ctx context.Context, u *user.User, req *link.UpdatePublicShareRequest, g *link.Grant) (*link.PublicShare, error)
	GetPublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference, sign bool) (*link.PublicShare, error)
	ListPublicShares(ctx context.Context, u *user.User, filters []*link.ListPublicSharesRequest_Filter, md *provider.ResourceInfo, sign bool) ([]*link.PublicShare, error)
	RevokePublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference) error
	GetPublicShareByToken(ctx context.Context, token string, auth *link.PublicShareAuthentication, sign bool) (*link.PublicShare, error)
}

// CreateSignature calculates a signature for a public share.
func CreateSignature(token, pw string, expiration time.Time) string {
	h := sha256.New()
	h.Write([]byte(pw))
	key := make([]byte, 0, 32)
	key = h.Sum(key)

	mac := hmac.New(sha512.New512_256, key)
	mac.Write([]byte(token + "|" + expiration.Format(time.RFC3339)))

	sig := make([]byte, 0, 32)
	sig = mac.Sum(sig)

	return hex.EncodeToString(sig)
}

// AddSignature augments a public share with a signature.
// The signature has a validity of 30 minutes.
func AddSignature(share *link.PublicShare, pw string) {
	expiration := time.Now().Add(time.Minute * 30)
	sig := CreateSignature(share.Token, pw, expiration)
	share.Signature = &link.ShareSignature{
		Signature: sig,
		SignatureExpiration: &typesv1beta1.Timestamp{
			Seconds: uint64(expiration.UnixNano() / 1000000000),
			Nanos:   uint32(expiration.UnixNano() % 1000000000),
		},
	}
}
