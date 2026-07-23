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
	"strings"

	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
	"github.com/pkg/errors"
)

// DefaultEntry is a default ACL entry that must or may exist on every path in a
// space, independent of any particular node. The full-namespace and per-space
// jobs turn each DefaultEntry into an ExpectedACL at every node they visit.
type DefaultEntry struct {
	// Entry is the resolved ACL entry (templates already substituted).
	Entry *acl.Entry
	// Enforcement is "must" or "may".
	Enforcement Enforcement
}

// DefaultACLs computes the default ACL entries for space from the configured
// path-prefix rules. It returns the entries of the rule whose prefix is a path
// prefix of the space root. Prefixes may not overlap, so at most one rule
// matches; the qualifier templates {owner} and {project} are resolved against
// the space. Returns nil if no rule matches.
func (c *Config) DefaultACLs(space *Space) ([]DefaultEntry, error) {
	if space == nil {
		return nil, errors.New("reconciliation: nil space")
	}
	switch space.Type {
	case SpaceTypePersonal, SpaceTypeProject:
	default:
		return nil, errors.Errorf("reconciliation: space %q has invalid type %q", space.ID, space.Type)
	}

	for i := range c.PathPrefixes {
		rule := &c.PathPrefixes[i]
		if !pathHasPrefix(space.Root, rule.Prefix) {
			continue
		}
		out := make([]DefaultEntry, 0, len(rule.DefaultACLs))
		for j := range rule.DefaultACLs {
			d := &rule.DefaultACLs[j]
			qualifier, err := resolveTemplate(d.Qualifier, space)
			if err != nil {
				return nil, errors.Wrapf(err, "reconciliation: path_prefix[%d].default_acl[%d]", i, j)
			}
			out = append(out, DefaultEntry{
				Entry: &acl.Entry{
					Type:        d.Type,
					Qualifier:   qualifier,
					Permissions: d.Permissions,
				},
				Enforcement: d.Enforcement,
			})
		}
		return out, nil
	}
	return nil, nil
}

// pathHasPrefix reports whether path lies at or under prefix, comparing whole
// path components so that "/eos/user" is a prefix of "/eos/user/j/jdoe" but not
// of "/eos/username". An empty prefix matches any path.
func pathHasPrefix(path, prefix string) bool {
	path = strings.TrimRight(path, "/")
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		return true
	}
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// resolveTemplate substitutes the {owner} and {project} placeholders in a
// qualifier with the space's owner and project. It is an error to use a
// placeholder the space does not populate.
func resolveTemplate(qualifier string, space *Space) (string, error) {
	if strings.Contains(qualifier, "{owner}") {
		if space.Owner == "" {
			return "", errors.Errorf("qualifier %q uses {owner} but space has no owner", qualifier)
		}
		qualifier = strings.ReplaceAll(qualifier, "{owner}", space.Owner)
	}
	if strings.Contains(qualifier, "{project}") {
		if space.Project == "" {
			return "", errors.Errorf("qualifier %q uses {project} but space has no project", qualifier)
		}
		qualifier = strings.ReplaceAll(qualifier, "{project}", space.Project)
	}
	return qualifier, nil
}
