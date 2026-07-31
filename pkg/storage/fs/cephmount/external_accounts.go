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

package cephmount

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// External accounts are unknown to the system: they have no uid, so the kernel
// cannot decide what they may do. Operations on their behalf run as the service
// account configured in external_accounts_user_*, and this file decides what
// that account is allowed to do on their behalf, from the grants stored in the
// xattrs by AddGrant.

// externalUser returns the user from ctx when it is an external account.
func externalUser(ctx context.Context) (*userv1beta1.User, bool) {
	u, ok := appctx.ContextGetUser(ctx)
	if !ok || u.Id == nil {
		return nil, false
	}
	if !utils.IsExternalUser(u) {
		return nil, false
	}
	return u, true
}

// externalAccountGrant returns the permissions granted to account on a
// chroot-relative path. The resource is checked first, then each ancestor up to
// the chroot root, so a grant on a directory covers everything below it. The
// xattrs are read with the driver's own privileges: the service account cannot
// be trusted to read them, and may not even be able to.
func (fs *cephmountfs) externalAccountGrant(account, chrootPath string) (*provider.ResourcePermissions, bool) {
	key := xattrLwShare + account
	for p := filepath.Clean(chrootPath); ; p = filepath.Dir(p) {
		value, err := xattr.Get(filepath.Join(fs.chrootDir, p), key)
		if err == nil {
			return fs.aclStringToPermissions(string(value)), true
		}
		if p == "." || p == string(filepath.Separator) {
			return nil, false
		}
	}
}

// authorizeExternal denies an operation when the user is an external account
// that has no grant covering chrootPath, or a grant that does not allow it. For
// every other user it does nothing: the operation runs under their own uid and
// the kernel enforces access, as it always has.
func (fs *cephmountfs) authorizeExternal(ctx context.Context, chrootPath, operation string, allowed func(*provider.ResourcePermissions) bool) error {
	u, ok := externalUser(ctx)
	if !ok {
		return nil
	}

	log := appctx.GetLogger(ctx)
	account := u.Id.OpaqueId

	perms, found := fs.externalAccountGrant(account, chrootPath)
	if !found {
		log.Debug().Str("account", account).Str("path", chrootPath).Str("operation", operation).
			Msg("cephmount: denying external account without a grant")
		return errtypes.PermissionDenied(fmt.Sprintf("cephmount: %s is not shared with %s", chrootPath, account))
	}
	if !allowed(perms) {
		log.Debug().Str("account", account).Str("path", chrootPath).Str("operation", operation).
			Msg("cephmount: denying external account, grant does not allow the operation")
		return errtypes.PermissionDenied(fmt.Sprintf("cephmount: %s not allowed on %s for %s", operation, chrootPath, account))
	}
	return nil
}

// The permissions an operation requires from the grant.
func canStat(p *provider.ResourcePermissions) bool     { return p.Stat }
func canGetPath(p *provider.ResourcePermissions) bool  { return p.GetPath }
func canGetQuota(p *provider.ResourcePermissions) bool { return p.GetQuota }
func canList(p *provider.ResourcePermissions) bool     { return p.ListContainer }
func canDownload(p *provider.ResourcePermissions) bool { return p.InitiateFileDownload }
func canUpload(p *provider.ResourcePermissions) bool   { return p.InitiateFileUpload }
func canCreate(p *provider.ResourcePermissions) bool   { return p.CreateContainer }
func canDelete(p *provider.ResourcePermissions) bool   { return p.Delete }
func canMove(p *provider.ResourcePermissions) bool     { return p.Move }
func canListGrants(p *provider.ResourcePermissions) bool {
	return p.ListGrants
}
func canAddGrant(p *provider.ResourcePermissions) bool { return p.AddGrant }
func canRemoveGrant(p *provider.ResourcePermissions) bool {
	return p.RemoveGrant
}
func canListRevisions(p *provider.ResourcePermissions) bool {
	return p.ListFileVersions
}
func canRestoreRevision(p *provider.ResourcePermissions) bool {
	return p.RestoreFileVersion
}

// ensureServiceAccountAccess gives the external accounts service account the
// POSIX access it needs to reach chrootPath for a sharee whose own uid does not
// exist, and without which the kernel refuses before reva's check is reached:
// the granted permissions on the shared resource, and rwx on the project root
// above it. The rest is assumed provisioned — world-readable directories down
// to the project roots, which are closed (other::--x) with named entries for
// the accounts let in. This adds the service account's on the first external
// share, as provisioning does for the others.
func (fs *cephmountfs) ensureServiceAccountAccess(ctx context.Context, chrootPath string, perms *provider.ResourcePermissions) error {
	fullPath := filepath.Join(fs.chrootDir, chrootPath)
	info, err := os.Stat(fullPath)
	if err != nil {
		return errors.Wrap(err, "cephmount: failed to stat path")
	}

	// The service account needs at least read and search on the shared resource
	// itself, whatever the sharee is allowed to do with it.
	acl := fs.permissionsToACLString(perms)
	if !strings.Contains(acl, "r") {
		acl = "r" + acl[1:]
	}
	if info.IsDir() && !strings.Contains(acl, "x") {
		acl = acl[:2] + "x"
	}
	if err := fs.setServiceACL(ctx, fullPath, acl, info.IsDir()); err != nil {
		return err
	}

	if root, ok := fs.spaceRootFor(chrootPath); ok {
		if err := fs.setServiceACL(ctx, filepath.Join(fs.chrootDir, root), "rwx", false); err != nil {
			return err
		}
	}
	return nil
}

// spaceRootFor returns the space (project) root a chroot-relative path lives
// under: its first SpaceDepth components (c/<project> in the canonical layout).
// The second return is false when the path is at or above space-root depth,
// where there is no space root to grant on, and when the storage provider did
// not tell us where spaces start: the chroot root is never a space root, and
// granting the service account rwx on it would open the whole mount.
func (fs *cephmountfs) spaceRootFor(chrootPath string) (string, bool) {
	if fs.conf.SpaceDepth <= 0 {
		return "", false
	}
	parts := strings.Split(filepath.Clean(chrootPath), string(filepath.Separator))
	if len(parts) <= fs.conf.SpaceDepth {
		return "", false
	}
	return filepath.Join(parts[:fs.conf.SpaceDepth]...), true
}

// removeServiceAccountAccess drops the service account's ACL from a resource
// that is no longer shared with any external account. The search rights on the
// ancestors are left in place: other shares below them may still need those, and
// they grant nothing beyond traversal.
func (fs *cephmountfs) removeServiceAccountAccess(ctx context.Context, chrootPath string) error {
	fullPath := filepath.Join(fs.chrootDir, chrootPath)
	info, err := os.Stat(fullPath)
	if err != nil {
		return errors.Wrap(err, "cephmount: failed to stat path")
	}

	entry := fmt.Sprintf("u:%d", fs.conf.ExternalAccountsUserUID)
	args := []string{"-x", entry}
	if info.IsDir() {
		args = append(args, "-x", "d:"+entry, "-R")
	}
	args = append(args, fullPath)

	return fs.runSetfacl(ctx, args, fullPath)
}

// hasExternalAccountGrants reports whether any external account still holds a
// grant on the resource. The error must not be mistaken for a false: the caller
// revokes access on it.
func (fs *cephmountfs) hasExternalAccountGrants(fullPath string) (bool, error) {
	names, err := xattr.List(fullPath)
	if err != nil {
		return false, errors.Wrap(err, "cephmount: failed to list xattrs")
	}
	for _, name := range names {
		if strings.HasPrefix(name, xattrLwShare) {
			return true, nil
		}
	}
	return false, nil
}

// setServiceACL grants the service account acl on fullPath. Directories also get
// a default entry so that new children inherit it, and are applied recursively,
// matching what the setfacl path does for regular grants.
func (fs *cephmountfs) setServiceACL(ctx context.Context, fullPath, acl string, recursive bool) error {
	entry := fmt.Sprintf("u:%d:%s", fs.conf.ExternalAccountsUserUID, acl)
	args := []string{"-m", entry}
	if recursive {
		args = append(args, "-m", "d:"+entry, "-R")
	}
	args = append(args, fullPath)

	return fs.runSetfacl(ctx, args, fullPath)
}

func (fs *cephmountfs) runSetfacl(ctx context.Context, args []string, fullPath string) error {
	log := appctx.GetLogger(ctx)

	cmd := exec.CommandContext(ctx, "setfacl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Err(err).Str("path", fullPath).Strs("args", args).Str("output", string(output)).
			Msg("cephmount: setfacl for the external accounts service user failed")
		return errors.Wrapf(err, "cephmount: setfacl failed: %s", string(output))
	}

	log.Debug().Str("path", fullPath).Strs("args", args).
		Msg("cephmount: updated the external accounts service user ACL")
	return nil
}
