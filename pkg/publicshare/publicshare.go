// Copyright 2018-2019 CERN
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

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
)

const (
	// ACLModeReadOnly specifies that the share is read-only.
	ACLModeReadOnly ACLMode = "read-only"

	// ACLModeReadWrite specifies that the share is read-writable.
	ACLModeReadWrite ACLMode = "read-write"

	// ACLTypeDirectory specifies that the share points to a directory.
	ACLTypeDirectory ACLType = "directory"

	// ACLTypeFile specifies that the share points to a file.
	ACLTypeFile ACLType = "file"
)

type (
	// Manager manipulates public shares.
	Manager interface {
		CreatePublicShare(ctx context.Context, u *authv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo, a *ACL) (*PublicShare, error)
		UpdatePublicShare(ctx context.Context, u *authv0alphapb.User, id string, up *UpdatePolicy, a *ACL) (*PublicShare, error)
		GetPublicShare(ctx context.Context, u *authv0alphapb.User, id string) (*PublicShare, error)
		ListPublicShares(ctx context.Context, u *authv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo) ([]*PublicShare, error)
		RevokePublicShare(ctx context.Context, u *authv0alphapb.User, id string) error
		GetPublicShareByToken(ctx context.Context, token string) (*PublicShare, error)
	}

	// PublicShare represents a public share.
	PublicShare struct {
		ID          string
		Token       string
		Filename    string
		Modified    uint64
		Owner       string
		DisplayName string
		ACL         *ACL
	}

	// ACL is the the acl to use when creating or updating public shares.
	ACL struct {
		Password   string
		Expiration uint64
		SetMode    bool
		Mode       ACLMode
		Type       ACLType
	}

	// UpdatePolicy specifies which attributes to update when calling UpdateACL.
	UpdatePolicy struct {
		SetPassword   bool
		SetExpiration bool
		SetMode       bool
	}

	// ACLMode represents the mode for the share (read-only, read-write, ...)
	ACLMode string

	// ACLType represents the type of file the share points to (file, directory, ...)
	ACLType string
)

/*
AuthenticatePublicShare(ctx context.Context, token, password string) (*PublicShare, error)
	IsPublicShareProtected(ctx context.Context, token string) (bool, error)
*/
