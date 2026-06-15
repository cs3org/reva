// Copyright 2018-2026 CERN
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

package sharehierarchy

import (
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// PermLevel represents a permission level in the hierarchy ordering.
// It exists because raw OCS uint8 values cannot be compared directly for ordering:
// OCS uses 0 for denial, which is numerically smallest but semantically most powerful.
// PermLevel provides a correctly-ordered integer so that p1 > p2 means "p1 is more powerful".
type PermLevel int

const (
	// PermRead represents read-only access (stat, list, download).
	PermRead PermLevel = 1
	// PermRW represents read-write access (read + upload, create, delete, move).
	PermRW PermLevel = 2
	// PermDeny represents an active denial — an explicit ACL entry that blocks access.
	// This is the most powerful level in the ordering (D > RW > R).
	// It is distinct from the absence of a share: PermDeny corresponds to a DB record
	// with permissions=0, whereas no share simply means no entry exists.
	PermDeny PermLevel = 3
)

func (p PermLevel) String() string {
	switch p {
	case PermRead:
		return "R"
	case PermRW:
		return "RW"
	case PermDeny:
		return "D"
	default:
		return "unknown"
	}
}

// PermLevelFromCS3 converts CS3 ResourcePermissions to the PermLevel ordering.
// An empty ResourcePermissions (all false) maps to PermDeny — an active denial.
func PermLevelFromCS3(p *provider.ResourcePermissions) PermLevel {
	if p == nil || isEmptyPermissions(p) {
		return PermDeny
	}
	if p.InitiateFileUpload || p.CreateContainer {
		return PermRW
	}
	return PermRead
}

func isEmptyPermissions(p *provider.ResourcePermissions) bool {
	return !p.AddGrant &&
		!p.CreateContainer &&
		!p.Delete &&
		!p.GetPath &&
		!p.GetQuota &&
		!p.InitiateFileDownload &&
		!p.InitiateFileUpload &&
		!p.ListContainer &&
		!p.ListFileVersions &&
		!p.ListGrants &&
		!p.ListRecycle &&
		!p.Move &&
		!p.PurgeRecycle &&
		!p.RemoveGrant &&
		!p.RestoreFileVersion &&
		!p.RestoreRecycleItem &&
		!p.Stat &&
		!p.UpdateGrant &&
		!p.DenyGrant
}
