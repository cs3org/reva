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

package acl

import (
	"errors"
	"fmt"
	"strings"
)

// The ACLs represent a delimiter separated list of ACL entries.
type ACLs struct {
	Entries   []*Entry
	delimiter string
}

var (
	errInvalidACL = errors.New("invalid acl")
)

const (
	// LongTextForm contains one ACL entry per line.
	LongTextForm = "\n"
	// ShortTextForm is a sequence of ACL entries separated by commas, and is used for input.
	ShortTextForm = ","

	// TypeUser indicates the qualifier identifies a user
	TypeUser = "u"
	// TypeGroup indicates the qualifier identifies a group
	TypeGroup = "egroup"
)

// Parse parses an acl string with the given delimiter (LongTextForm or ShortTextForm)
func Parse(acls string, delimiter string) (*ACLs, error) {
	tokens := strings.Split(acls, delimiter)
	entries := []*Entry{}
	for _, t := range tokens {
		// ignore empty lines and comments
		if t == "" || isComment(t) {
			continue
		}
		entry, err := ParseEntry(t)
		if err != nil {
			return nil, err
		}
		// for now we ignore default / empty qualifiers
		// if entry.Qualifier == "" {
		//	continue
		// }
		entries = append(entries, entry)
	}

	return &ACLs{Entries: entries, delimiter: delimiter}, nil
}

func isComment(line string) bool {
	return strings.HasPrefix(line, "#")
}

// Serialize always serializes to short text form
func (m *ACLs) Serialize() string {
	sysACL := []string{}
	for _, e := range m.Entries {
		sysACL = append(sysACL, e.serialize())
	}
	return strings.Join(sysACL, ShortTextForm)
}

// DeleteEntry removes an entry uniquely identified by acl type and qualifier
func (m *ACLs) DeleteEntry(aclType string, qualifier string) {
	for i, e := range m.Entries {
		if e.Qualifier == qualifier && e.Type == aclType {
			m.Entries = append(m.Entries[:i], m.Entries[i+1:]...)
			return
		}
	}
}

// SetEntry replaces the permissions of an entry with the given set
func (m *ACLs) SetEntry(aclType string, qualifier string, permissions string) error {
	if aclType == "" || permissions == "" {
		return errInvalidACL
	}
	m.DeleteEntry(aclType, qualifier)
	entry := &Entry{
		Type:        aclType,
		Qualifier:   qualifier,
		Permissions: permissions,
	}
	m.Entries = append(m.Entries, entry)
	return nil
}

// The Entry of an ACL is represented as three colon separated fields:
type Entry struct {
	// an ACL entry tag type: user, group, mask or other. comments start with #
	Type string
	// an ACL entry qualifier
	Qualifier string
	// and the discretionary access permissions
	Permissions string
}

// ParseEntry parses a single ACL
func ParseEntry(singleSysACL string) (*Entry, error) {
	tokens := strings.Split(singleSysACL, ":")
	if len(tokens) != 3 {
		return nil, errInvalidACL
	}

	return &Entry{
		Type:        tokens[0],
		Qualifier:   tokens[1],
		Permissions: tokens[2],
	}, nil
}

// CitrineSerialize serializes an ACL entry for citrine EOS ACLs
func (a *Entry) CitrineSerialize() string {
	return fmt.Sprintf("%s:%s=%s", a.Type, a.Qualifier, a.Permissions)
}

func (a *Entry) serialize() string {
	return strings.Join([]string{a.Type, a.Qualifier, a.Permissions}, ":")
}
