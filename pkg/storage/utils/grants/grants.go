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

package grants

import (
	"errors"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// GetACLPerm generates a string representation of CS3APIs' ResourcePermissions
// TODO(labkode): fine grained permission controls.
func GetACLPerm(set *provider.ResourcePermissions) (string, error) {
	var b strings.Builder

	if set.Stat || set.InitiateFileDownload {
		b.WriteString("r")
	}
	if set.CreateContainer || set.InitiateFileUpload || set.Delete || set.Move {
		b.WriteString("w")
	}
	if set.ListContainer {
		b.WriteString("x")
	}

	if set.Delete {
		b.WriteString("+d")
	} else {
		b.WriteString("!d")
	}

	// TODO sharing
	// TODO trash
	// TODO versions
	return b.String(), nil
}

// GetGrantPermissionSet converts CSEAPIs' ResourcePermissions from a string
// TODO(labkode): add more fine grained controls.
// EOS acls are a mix of ACLs and POSIX permissions. More details can be found in
// https://github.com/cern-eos/eos/blob/master/doc/configuration/permission.rst
// TODO we need to evaluate all acls in the list at once to properly forbid (!) and overwrite (+) permissions
// This is ugly, because those are actually negative permissions ...
func GetGrantPermissionSet(mode string) *provider.ResourcePermissions {

	// TODO also check unix permissions for read access
	p := &provider.ResourcePermissions{}
	// r
	if strings.Contains(mode, "r") {
		p.Stat = true
		p.InitiateFileDownload = true
	}
	// w
	if strings.Contains(mode, "w") {
		p.CreateContainer = true
		p.InitiateFileUpload = true
		p.Delete = true
		if p.InitiateFileDownload {
			p.Move = true
		}
	}
	if strings.Contains(mode, "wo") {
		p.CreateContainer = true
		//	p.InitiateFileUpload = false // TODO only when the file exists
		p.Delete = false
	}
	if strings.Contains(mode, "!d") {
		p.Delete = false
	} else if strings.Contains(mode, "+d") {
		p.Delete = true
	}
	// x
	if strings.Contains(mode, "x") {
		p.ListContainer = true
	}

	// sharing
	// TODO AddGrant
	// TODO ListGrants
	// TODO RemoveGrant
	// TODO UpdateGrant

	// trash
	// TODO ListRecycle
	// TODO RestoreRecycleItem
	// TODO PurgeRecycle

	// versions
	// TODO ListFileVersions
	// TODO RestoreFileVersion

	// ?
	// TODO GetPath
	// TODO GetQuota
	return p
}

// GetACLType returns a char representation of the type of grantee
func GetACLType(gt provider.GranteeType) (string, error) {
	switch gt {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		return "u", nil
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		return "g", nil
	default:
		return "", errors.New("no eos acl for grantee type: " + gt.String())
	}
}

// GetGranteeType returns the grantee type from a char
func GetGranteeType(aclType string) provider.GranteeType {
	switch aclType {
	case "u":
		return provider.GranteeType_GRANTEE_TYPE_USER
	case "g":
		return provider.GranteeType_GRANTEE_TYPE_GROUP
	default:
		return provider.GranteeType_GRANTEE_TYPE_INVALID
	}
}
