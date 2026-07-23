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

package reconciliation

import (
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
	"github.com/pkg/errors"
)

// Enforcement decides how the reconciler treats a default ACL entry that
// diverges from what is on the storage.
type Enforcement string

const (
	// EnforcementMay means the entry is allowed to be present but is not
	// required. The reconciler never adds it and never removes it: a "may"
	// entry on disk is left untouched. Used for the global defaults
	// (cbackeosro, cboxexternal) that may live anywhere.
	EnforcementMay Enforcement = "may"
	// EnforcementMust means the entry has to be present everywhere in scope.
	// The reconciler adds it when missing and corrects it when its permissions
	// differ, and never removes it. Used for the personal-space owner and the
	// project reader/writer/admin egroups.
	EnforcementMust Enforcement = "must"
)

// DefaultScanner is the default NamespaceScanner used for the full-namespace
// sweep. It shells out to the eos-ns-inspect binary.
const DefaultScanner = "eos-nsinspect-binary"

// Config configures the reconciliation engine. It is decoded from the job's own
// configuration section.
type Config struct {
	// DryRun, when set, makes every job compute and report the ACL changes it
	// would make without applying any of them.
	DryRun bool `mapstructure:"dry_run"`
	// Scanner selects the registered NamespaceScanner used by the
	// full-namespace sweep (level 3). Defaults to DefaultScanner.
	Scanner string `mapstructure:"scanner"`
	// PathPrefixes maps filesystem path prefixes to the default ACL entries
	// that apply under them. A space is governed by the single rule whose prefix
	// is a path prefix of its root. Prefixes may not overlap, so at most one rule
	// matches a space. See Config.DefaultACLs.
	PathPrefixes []PathPrefixRule `mapstructure:"path_prefix"`
}

// PathPrefixRule associates a path prefix with a set of default ACL entries.
type PathPrefixRule struct {
	// Prefix is the filesystem path prefix the rule applies to, e.g.
	// "/eos/user" or "/eos/project".
	Prefix string `mapstructure:"prefix"`
	// DefaultACLs are the default entries that apply under Prefix.
	DefaultACLs []DefaultACLRule `mapstructure:"default_acl"`
}

// DefaultACLRule is a single default ACL entry and how strictly it is enforced.
// The qualifier may contain the templates "{owner}" and "{project}", resolved
// per space when the defaults are computed.
type DefaultACLRule struct {
	// Type is the ACL entry type: "u" (user), "egroup" (group) or "lw"
	// (lightweight). See package acl.
	Type string `mapstructure:"type"`
	// Qualifier identifies the grantee. May contain "{owner}" or "{project}".
	Qualifier string `mapstructure:"qualifier"`
	// Permissions is the EOS permission string, e.g. "rx", "rwx" or "rwx+d".
	Permissions string `mapstructure:"permissions"`
	// Enforcement is "must" or "may".
	Enforcement Enforcement `mapstructure:"enforcement"`
}

// ApplyDefaults implements cfg.Setter.
func (c *Config) ApplyDefaults() {
	if c.Scanner == "" {
		c.Scanner = DefaultScanner
	}
}

// Validate checks that the configuration is internally consistent. It is
// separate from ApplyDefaults so a caller can decode, default and validate in
// that order and surface a precise error.
func (c *Config) Validate() error {
	for i := range c.PathPrefixes {
		if err := c.PathPrefixes[i].validate(); err != nil {
			return errors.Wrapf(err, "reconciliation: path_prefix[%d]", i)
		}
	}
	return nil
}

func (r *PathPrefixRule) validate() error {
	if r.Prefix == "" {
		return errors.New("prefix must not be empty")
	}
	for j := range r.DefaultACLs {
		if err := r.DefaultACLs[j].validate(); err != nil {
			return errors.Wrapf(err, "default_acl[%d]", j)
		}
	}
	return nil
}

func (d *DefaultACLRule) validate() error {
	switch d.Type {
	case acl.TypeUser, acl.TypeGroup, acl.TypeLightweight:
	case "":
		return errors.New("type must not be empty")
	default:
		return errors.Errorf("invalid type %q", d.Type)
	}
	if d.Qualifier == "" {
		return errors.New("qualifier must not be empty")
	}
	if d.Permissions == "" {
		return errors.New("permissions must not be empty")
	}
	switch d.Enforcement {
	case EnforcementMay, EnforcementMust:
	case "":
		return errors.New("enforcement must not be empty")
	default:
		return errors.Errorf("invalid enforcement %q", d.Enforcement)
	}
	return nil
}
