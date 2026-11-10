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

package eos

import (
	"context"
	"iter"
	"path"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/reconciliation"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/cs3org/reva/v3/pkg/storage/fs/eos/acl"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/storage/fs/eos/nsdump"
	"github.com/cs3org/reva/v3/pkg/storage/utils/grants"
	"github.com/pkg/errors"
)

var _ reconciliation.ReconcilableFS = (*Eosfs)(nil)

// DumpNamespace reads the namespace under p with eos-ns-inspect. The paths are
// EOS paths, so they are not wrapped.
func (fs *Eosfs) DumpNamespace(ctx context.Context, p string) iter.Seq2[reconciliation.NSDumpInfo, error] {
	return func(yield func(reconciliation.NSDumpInfo, error) bool) {
		if len(fs.conf.NSInspectQuarkDBNodes) == 0 || fs.conf.NSInspectPasswordFile == "" {
			yield(reconciliation.NSDumpInfo{}, errors.New("eosfs: set ns_inspect_quarkdb_nodes and ns_inspect_password_file to dump the namespace"))
			return
		}
		inspect := nsdump.NSInspectConfig{
			Binary:       fs.conf.NSInspectBinary,
			QuarkDBNodes: fs.conf.NSInspectQuarkDBNodes,
			PasswordFile: fs.conf.NSInspectPasswordFile,
		}

		var dumper nsdump.NSDumpClient
		var conf any
		switch fs.conf.NSDumpDriver {
		case "chunked":
			dumper = &nsdump.EOSChunkedNSInspect{}
			conf = nsdump.EOSChunkedNSInspectConfig{NSInspectConfig: inspect, TmpDir: fs.conf.NSDumpTmpDir}
		case "memory":
			dumper = &nsdump.EOSMemoryNSInspect{}
			conf = inspect
		default:
			yield(reconciliation.NSDumpInfo{}, errors.Errorf("eosfs: nsdump_driver %q is not chunked or memory", fs.conf.NSDumpDriver))
			return
		}
		if err := dumper.Setup(conf); err != nil {
			yield(reconciliation.NSDumpInfo{}, err)
			return
		}

		for e, err := range dumper.Dump(p, 0) {
			if err != nil {
				yield(reconciliation.NSDumpInfo{}, err)
				return
			}
			info, ok := toNSDumpInfo(e)
			if !ok {
				continue
			}
			if !yield(info, nil) {
				return
			}
		}
	}
}

// toNSDumpInfo converts one entry of eos-ns-inspect into the file or folder the
// job compares. It is false for an entry to leave out of the dump: a version,
// another .sys. entry, or an ACL we cannot read.
func toNSDumpInfo(e nsdump.NameSpaceEntry) (reconciliation.NSDumpInfo, bool) {
	info := reconciliation.NSDumpInfo{Path: e.Path, ID: e.ID(), Type: provider.ResourceType_RESOURCE_TYPE_FILE}
	if e.EntryType() == nsdump.EntryTypeFolder {
		info.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}

	// a folder path ends with a separator, and path.Dir would keep the folder.
	clean := path.Clean(e.Path)
	if eosclient.IsVersionFolder(clean) {
		file := eosclient.GetFileFromVersionFolder(clean)
		// a version inside a version folder: a share never sets an ACL on it.
		if file == clean {
			return info, false
		}
		// AddGrant writes the ACL of a file on its version folder too, because
		// an overwrite wipes the one on the file.
		info.ACLsOf = file
	} else if strings.HasPrefix(path.Base(clean), ".sys.") {
		return info, false
	}

	acls, err := acl.Parse(e.XattrSysAcl, acl.ShortTextForm)
	if err != nil {
		// an ACL we cannot read is one we could only break.
		return info, false
	}
	for _, a := range acls.Entries {
		var t provider.GranteeType
		switch a.Type {
		case acl.TypeUser:
			t = provider.GranteeType_GRANTEE_TYPE_USER
		case acl.TypeGroup:
			t = provider.GranteeType_GRANTEE_TYPE_GROUP
		default:
			continue
		}
		info.ACLs = append(info.ACLs, &reconciliation.ACLEntry{
			Type:        t,
			Qualifier:   a.Qualifier,
			Permissions: sharehierarchy.PermLevelFromCS3(grants.GetGrantPermissionSet(a.Permissions)),
		})
	}
	return info, true
}

// AddNodeACL writes e on the node at p only. The permissions are the ones
// AddGrant writes for the same level, so a share and the job agree.
func (fs *Eosfs) AddNodeACL(ctx context.Context, p string, e reconciliation.ACLEntry) error {
	t, err := grants.GetACLType(e.Type)
	if err != nil {
		return err
	}
	var perms *provider.ResourcePermissions
	position := eosclient.StartPosition
	switch e.Permissions {
	case sharehierarchy.PermRead:
		perms = permissions.NewViewerRole().CS3ResourcePermissions()
	case sharehierarchy.PermRW:
		perms = permissions.NewEditorRole().CS3ResourcePermissions()
	case sharehierarchy.PermDeny:
		perms = &provider.ResourcePermissions{}
		// as in DenyGrant
		position = eosclient.EndPosition
	default:
		return errors.Errorf("eosfs: no eos permissions for level %s", e.Permissions)
	}
	eosPerms, err := grants.GetACLPerm(perms)
	if err != nil {
		return err
	}
	a := &acl.Entry{Type: t, Qualifier: e.Qualifier, Permissions: eosPerms}
	return fs.c.AddACL(ctx, getSystemAuth(), p, position, a, false)
}

// RemoveNodeACL removes the entry of the grantee of e from the node at p only.
func (fs *Eosfs) RemoveNodeACL(ctx context.Context, p string, e reconciliation.ACLEntry) error {
	t, err := grants.GetACLType(e.Type)
	if err != nil {
		return err
	}
	return fs.c.RemoveACL(ctx, getSystemAuth(), p, &acl.Entry{Type: t, Qualifier: e.Qualifier}, false)
}
