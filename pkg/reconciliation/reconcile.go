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

// Package reconciliation reconciles the share database against the ACLs stored
// on the storage. It runs as three independent jobs: detecting orphaned shares,
// correcting the ACLs on shared paths per space, and sweeping the whole
// namespace of every space. The engine is storage-driver agnostic: it reads
// shares from the database, resolves identities and mutates ACLs through the
// gateway (CS3), and only reaches driver-specific code through the
// NamespaceScanner interface for the full-namespace sweep.
package reconciliation

import (
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
)

// SpaceType is the kind of a storage space.
type SpaceType string

const (
	// SpaceTypePersonal is a personal space, one per user account.
	SpaceTypePersonal SpaceType = "personal"
	// SpaceTypeProject is a project space, shared for collaborative work.
	SpaceTypeProject SpaceType = "project"
)

// Space is a storage space to reconcile. Spaces are disjoint: reconciliation
// never crosses a space boundary.
type Space struct {
	// ID is the CS3 space id.
	ID string
	// Type is the space kind. Always SpaceTypePersonal or SpaceTypeProject for
	// a real space.
	Type SpaceType
	// StorageID is the storage provider (EOS instance) hosting the space.
	StorageID string
	// Root is the filesystem path of the space root.
	Root string
	// Owner is the owner's username. Set for personal spaces.
	Owner string
	// Project is the project name. Set for project spaces.
	Project string
}

// RecipientKind classifies the target of a share, which determines how its ACL
// is encoded on the storage.
type RecipientKind int

const (
	// RecipientUser is a CERN user account. Goes into the native ACLs.
	RecipientUser RecipientKind = iota
	// RecipientGroup is a group. Goes into the native ACLs.
	RecipientGroup
	// RecipientLightweight is an external account. Does not go into the native
	// ACLs but into a dedicated sys.reva.lwshare.<email> attribute, handled by
	// the storage driver.
	RecipientLightweight
)

// String returns a human readable name for the recipient kind.
func (k RecipientKind) String() string {
	switch k {
	case RecipientUser:
		return "user"
	case RecipientGroup:
		return "group"
	case RecipientLightweight:
		return "lightweight"
	default:
		return "unknown"
	}
}

// Recipient is the target of a share.
type Recipient struct {
	// Kind is the recipient classification.
	Kind RecipientKind
	// ID is the username, group name, or external account email, depending on
	// Kind.
	ID string
}

// ExpectedACL is an ACL entry that should exist at Path, as reconstructed from
// the shares and default rules for a space.
type ExpectedACL struct {
	// Path is the filesystem path the entry applies to.
	Path string
	// Entry is the ACL entry (type, qualifier, permissions).
	Entry *acl.Entry
	// Enforcement decides how a divergence is treated: a "must" entry is added
	// or corrected when missing or wrong, a "may" entry is left untouched
	// whether present or absent.
	Enforcement Enforcement
}

// ActionKind is the type of an ACL mutation.
type ActionKind int

const (
	// ActionAdd adds an ACL entry that is missing.
	ActionAdd ActionKind = iota
	// ActionRemove removes an ACL entry that should not be present.
	ActionRemove
	// ActionUpdate changes the permissions of an existing ACL entry.
	ActionUpdate
)

// String returns a human readable name for the action kind.
func (k ActionKind) String() string {
	switch k {
	case ActionAdd:
		return "add"
	case ActionRemove:
		return "remove"
	case ActionUpdate:
		return "update"
	default:
		return "unknown"
	}
}

// Action is a single ACL mutation on a path, the unit the applier executes
// through the CS3 grant API.
type Action struct {
	// Kind is the mutation type.
	Kind ActionKind
	// Path is the filesystem path to mutate.
	Path string
	// Entry is the ACL entry to add, remove or update.
	Entry *acl.Entry
}

// Plan is the ordered set of ACL mutations that brings the observed ACLs in
// line with the expected ones. Actions are ordered so that parent paths are
// mutated before their children.
type Plan struct {
	Actions []Action
}

// Len returns the number of actions in the plan.
func (p *Plan) Len() int { return len(p.Actions) }

// Outcome records the result of applying (or, in dry-run, simulating) a Plan.
type Outcome struct {
	// Applied lists the actions that were carried out, or that would have been
	// carried out when DryRun is true.
	Applied []Action
	// DryRun reports whether the outcome is a simulation.
	DryRun bool
}
