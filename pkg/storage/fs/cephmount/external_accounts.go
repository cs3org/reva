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
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// External accounts are unknown to the system: they have no uid, so the kernel
// cannot decide what they may do. Operations on their behalf run as the service
// account configured in external_accounts_user_*. This file holds the grants
// those accounts are given, stored in xattrs because there is no uid to put in
// a POSIX ACL, and decides from them what the service account may do. Public
// links and OCM shares carry their role in the token instead. As on eos, the
// service account itself gets rwx wherever an external account goes.

const (
	xattrACLAccess = "system.posix_acl_access"
	aclTagUser     = 0x02
	aclTagMask     = 0x10
)

// externalAccountQualifier returns the xattr qualifier for a lightweight or
// federated grantee. Those accounts only exist inside reva and must never be
// resolved as local users.
func externalAccountQualifier(g *provider.Grantee) (string, bool) {
	if g == nil || g.Type != provider.GranteeType_GRANTEE_TYPE_USER {
		return "", false
	}
	id := g.GetUserId()
	if id == nil {
		return "", false
	}
	switch id.Type {
	case userv1beta1.UserType_USER_TYPE_LIGHTWEIGHT, userv1beta1.UserType_USER_TYPE_FEDERATED:
		return id.OpaqueId, true
	default:
		return "", false
	}
}

// addExternalAccountGrant stores a grant to an external account as an xattr.
// Unlike the setfacl path this is not recursive: the attribute is only read back
// by reva, on the shared resource itself.
func (fs *cephmountfs) addExternalAccountGrant(ctx context.Context, path, qualifier string, perms *provider.ResourcePermissions) error {
	fullPath := filepath.Join(fs.chrootDir, path)
	key := xattrExtShare + qualifier
	value := fs.permissionsToACLString(perms)

	if err := xattr.Set(fullPath, key, []byte(value)); err != nil {
		return errors.Wrapf(err, "cephmount: failed to set xattr %s", key)
	}

	// The grant is only meaningful to reva. The service account acting for the
	// sharee still needs the kernel to let it through.
	if err := fs.ensureServiceAccountAccess(ctx, path); err != nil {
		return err
	}

	appctx.GetLogger(ctx).Debug().
		Str("path", fullPath).
		Str("xattr", key).
		Str("permissions", value).
		Msg("cephmount: stored grant for external account")

	return nil
}

// removeExternalAccountGrant drops the xattr holding the grant. The service
// account keeps its ACL: other external accounts may still go through it.
func (fs *cephmountfs) removeExternalAccountGrant(ctx context.Context, path, qualifier string) error {
	fullPath := filepath.Join(fs.chrootDir, path)
	key := xattrExtShare + qualifier

	if err := xattr.Remove(fullPath, key); err != nil && !errors.Is(err, xattr.ENOATTR) {
		return errors.Wrapf(err, "cephmount: failed to remove xattr %s", key)
	}

	appctx.GetLogger(ctx).Debug().
		Str("path", fullPath).
		Str("xattr", key).
		Msg("cephmount: removed grant for external account")

	return nil
}

// listExternalAccountGrants returns the grants stored in the xattrs of fullPath.
// A resource without any is not an error, but a failure to read them is: an
// empty list must never stand for grants that could not be read.
func (fs *cephmountfs) listExternalAccountGrants(ctx context.Context, fullPath string) ([]*provider.Grant, error) {
	log := appctx.GetLogger(ctx)

	names, err := xattr.List(fullPath)
	if err != nil {
		return nil, errors.Wrapf(err, "cephmount: failed to list the xattrs of %s", fullPath)
	}

	var glist []*provider.Grant
	for _, name := range names {
		if !strings.HasPrefix(name, xattrExtShare) {
			continue
		}
		buf, err := xattr.Get(fullPath, name)
		if err != nil {
			if errors.Is(err, xattr.ENOATTR) {
				// The grant was removed between the listing and the read.
				log.Debug().Str("xattr", name).Msg("cephmount: external account grant vanished while listing")
				continue
			}
			return nil, errors.Wrapf(err, "cephmount: failed to read the xattr %s", name)
		}
		glist = append(glist, &provider.Grant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{
					// Only the account name is stored, so the idp is left empty and
					// federated accounts are reported as lightweight ones.
					Type:     userv1beta1.UserType_USER_TYPE_LIGHTWEIGHT,
					OpaqueId: strings.TrimPrefix(name, xattrExtShare),
				}},
			},
			Permissions: fs.aclStringToPermissions(string(buf)),
		})
	}
	return glist, nil
}

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
	key := xattrExtShare + account
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

// shareRolePermissions returns the role of the public link or OCM share the user came in through.
func shareRolePermissions(u *userv1beta1.User) (*provider.ResourcePermissions, bool) {
	if role, ok := utils.HasPublicShareRole(u); ok {
		switch role {
		case "editor":
			return permissions.NewEditorRole().CS3ResourcePermissions(), true
		case "uploader":
			return permissions.NewUploaderRole().CS3ResourcePermissions(), true
		}
		return permissions.NewViewerRole().CS3ResourcePermissions(), true
	}
	if role, ok := utils.HasOCMShareRole(u); ok {
		if role == "editor" {
			return permissions.NewEditorRole().CS3ResourcePermissions(), true
		}
		return permissions.NewViewerRole().CS3ResourcePermissions(), true
	}
	return nil, false
}

// externalPermissions returns what the external account u may do on a chroot-relative path.
func (fs *cephmountfs) externalPermissions(u *userv1beta1.User, chrootPath string) (*provider.ResourcePermissions, bool) {
	// The token scope already confines these to the shared resource.
	if perms, ok := shareRolePermissions(u); ok {
		return perms, true
	}
	return fs.externalAccountGrant(u.Id.OpaqueId, chrootPath)
}

// authorizeExternal denies an operation the external account may not do, and
// otherwise makes sure the service account it runs as can carry it out. For
// every other user it does nothing: the operation runs under their own uid and
// the kernel enforces access, as it always has.
func (fs *cephmountfs) authorizeExternal(ctx context.Context, chrootPath, operation string, allowed func(*provider.ResourcePermissions) bool) error {
	if err := fs.checkExternal(ctx, chrootPath, operation, allowed); err != nil {
		return err
	}
	if _, ok := externalUser(ctx); !ok {
		return nil
	}
	return fs.ensureServiceAccountAccess(ctx, chrootPath)
}

// checkExternal is authorizeExternal without touching the service account's access.
func (fs *cephmountfs) checkExternal(ctx context.Context, chrootPath, operation string, allowed func(*provider.ResourcePermissions) bool) error {
	u, ok := externalUser(ctx)
	if !ok {
		return nil
	}

	log := appctx.GetLogger(ctx)
	account := u.Id.OpaqueId

	// The driver resolves paths as root, and a symlink could lead out of the share.
	symlinked, err := fs.throughSymlink(chrootPath)
	if err != nil {
		return err
	}
	if symlinked {
		log.Debug().Str("account", account).Str("path", chrootPath).Str("operation", operation).
			Msg("cephmount: denying external account through a symlink")
		return errtypes.PermissionDenied(fmt.Sprintf("cephmount: %s goes through a symlink", chrootPath))
	}

	perms, found := fs.externalPermissions(u, chrootPath)
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

// ensureServiceAccountAccess gives the external accounts service account rwx on
// chrootPath, and below it for a directory, unless it has it already. A path
// that does not exist yet gets it on its parent alone. The space root above also
// gets rwx; the rest is assumed provisioned — world-readable directories down to
// the project roots, which are closed (other::--x) with named entries for the
// accounts let in.
func (fs *cephmountfs) ensureServiceAccountAccess(ctx context.Context, chrootPath string) error {
	p := filepath.Clean(chrootPath)
	recursive := true
	info, err := os.Lstat(filepath.Join(fs.chrootDir, p))
	if os.IsNotExist(err) {
		p, recursive = filepath.Dir(p), false
		info, err = os.Lstat(filepath.Join(fs.chrootDir, p))
	}
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return errors.Wrap(err, "cephmount: failed to stat path")
	}

	// Never open a whole space, nor follow a symlink as root.
	if !fs.insideSpace(p) || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	// A named entry does nothing for the owner.
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) == fs.conf.ExternalAccountsUserUID {
		return nil
	}

	fullPath := filepath.Join(fs.chrootDir, p)
	want := uint16(0b110)
	if info.IsDir() {
		want = 0b111
	}
	has, err := fs.serviceAccountHas(fullPath, want)
	if err != nil || has {
		return err
	}

	if err := fs.setServiceACL(ctx, fullPath, "rwx", recursive && info.IsDir()); err != nil {
		return err
	}
	if root, ok := fs.spaceRootFor(p); ok {
		if err := fs.setServiceACL(ctx, filepath.Join(fs.chrootDir, root), "rwx", false); err != nil {
			return err
		}
	}
	return nil
}

// serviceAccountHas reports whether the access ACL of fullPath gives the service account the want bits.
func (fs *cephmountfs) serviceAccountHas(fullPath string, want uint16) (bool, error) {
	buf, err := xattr.Get(fullPath, xattrACLAccess)
	if errors.Is(err, xattr.ENOATTR) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrapf(err, "cephmount: failed to read the ACL of %s", fullPath)
	}
	// A version header, then 8-byte little-endian entries: tag, perm, id.
	if len(buf) < 4 || (len(buf)-4)%8 != 0 {
		return false, errors.Errorf("cephmount: malformed ACL on %s", fullPath)
	}
	var perm, mask uint16 = 0, 0b111
	for e := buf[4:]; len(e) > 0; e = e[8:] {
		switch binary.LittleEndian.Uint16(e) {
		case aclTagUser:
			if binary.LittleEndian.Uint32(e[4:]) == uint32(fs.conf.ExternalAccountsUserUID) {
				perm = binary.LittleEndian.Uint16(e[2:])
			}
		case aclTagMask:
			mask = binary.LittleEndian.Uint16(e[2:])
		}
	}
	return perm&mask&want == want, nil
}

// insideSpace reports whether a chroot-relative path lies strictly below a space root.
func (fs *cephmountfs) insideSpace(chrootPath string) bool {
	p := filepath.Clean(chrootPath)
	return p != "." && filepath.IsLocal(p) && len(strings.Split(p, string(filepath.Separator))) > fs.conf.SpaceDepth
}

// throughSymlink reports whether a chroot-relative path, or the part of it that exists, resolves through a symlink.
func (fs *cephmountfs) throughSymlink(chrootPath string) (bool, error) {
	root, err := filepath.EvalSymlinks(fs.chrootDir)
	if err != nil {
		return false, errors.Wrap(err, "cephmount: failed to resolve the chroot")
	}
	for p := filepath.Clean(chrootPath); ; p = filepath.Dir(p) {
		fullPath := filepath.Join(fs.chrootDir, p)
		resolved, err := filepath.EvalSymlinks(fullPath)
		if err == nil {
			return resolved != filepath.Join(root, p), nil
		}
		if !os.IsNotExist(err) && !errors.Is(err, syscall.ENOTDIR) {
			return false, errors.Wrap(err, "cephmount: failed to resolve path")
		}
		if _, err := os.Lstat(fullPath); err == nil {
			return true, nil // a dangling symlink
		}
		if p == "." {
			return false, nil
		}
	}
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
