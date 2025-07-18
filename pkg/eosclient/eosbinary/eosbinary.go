// Copyright 2018-2024 CERN
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

package eosbinary

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	"github.com/cs3org/reva/v3/pkg/appctx"

	"github.com/cs3org/reva/v3/pkg/eosclient"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
	"github.com/cs3org/reva/v3/pkg/trace"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

func serializeAttribute(a *eosclient.Attribute) string {
	return fmt.Sprintf("%s.%s=%s", attrTypeToString(a.Type), a.Key, a.Val)
}

func attrTypeToString(at eosclient.AttrType) string {
	switch at {
	case eosclient.SystemAttr:
		return "sys"
	case eosclient.UserAttr:
		return "user"
	default:
		return "invalid"
	}
}

func isValidAttribute(a *eosclient.Attribute) bool {
	// validate that an attribute is correct.
	if (a.Type != eosclient.SystemAttr && a.Type != eosclient.UserAttr) || a.Key == "" {
		return false
	}
	return true
}

// Options to configure the Client.
type Options struct {
	// ForceSingleUserMode forces all connections to use only one user.
	// This is the case when access to EOS is done from FUSE under apache or www-data.
	ForceSingleUserMode bool

	// UseKeyTabAuth changes will authenticate requests by using an EOS keytab.
	UseKeytab bool

	// Whether to maintain the same inode across various versions of a file.
	// Requires extra metadata operations if set to true
	VersionInvariant bool

	// SingleUsername is the username to use when connecting to EOS.
	// Defaults to apache
	SingleUsername string

	// Location of the eos binary.
	// Default is /usr/bin/eos.
	EosBinary string

	// Location of the xrdcopy binary.
	// Default is /opt/eos/xrootd/bin/xrdcopy.
	XrdcopyBinary string

	// URL of the EOS MGM.
	// Default is root://eos-example.org
	URL string

	// Location on the local fs where to store reads.
	// Defaults to os.TempDir()
	CacheDirectory string

	// Keytab is the location of the EOS keytab file.
	Keytab string

	// SecProtocol is the comma separated list of security protocols used by xrootd.
	// For example: "sss, unix"
	// DEPRECATED
	// This variable is no longer used. Only sss and unix protocols are possible.
	// If UseKeytab is set to true the protocol will be set to "sss", else to "unix"
	SecProtocol string

	// TokenExpiry stores in seconds the time after which generated tokens will expire
	// Default is 3600
	TokenExpiry int
}

func (opt *Options) ApplyDefaults() {
	if opt.ForceSingleUserMode && opt.SingleUsername != "" {
		opt.SingleUsername = "apache"
	}

	if opt.EosBinary == "" {
		opt.EosBinary = "/usr/bin/eos"
	}

	if opt.XrdcopyBinary == "" {
		opt.XrdcopyBinary = "/opt/eos/xrootd/bin/xrdcopy"
	}

	if opt.URL == "" {
		opt.URL = "root://eos-example.org"
	}

	if opt.CacheDirectory == "" {
		opt.CacheDirectory = os.TempDir()
	}
}

// Client performs actions against a EOS management node (MGM).
// It requires the eos-client and xrootd-client packages installed to work.
type Client struct {
	opt *Options
	// Keep path -> version folder cache
	versionFolderCache *ttlcache.Cache
}

// New creates a new client with the given options.
func New(opt *Options) (*Client, error) {
	opt.ApplyDefaults()
	c := new(Client)
	c.opt = opt
	c.versionFolderCache = ttlcache.NewCache()
	c.versionFolderCache.SetTTL(24 * 31 * time.Hour)
	return c, nil
}

// executeXRDCopy executes xrdcpy commands and returns the stdout, stderr and return code.
func (c *Client) executeXRDCopy(ctx context.Context, cmdArgs []string) (string, string, error) {
	log := appctx.GetLogger(ctx)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}

	cmd := exec.CommandContext(ctx, c.opt.XrdcopyBinary, cmdArgs...)
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	cmd.Env = []string{
		"EOS_MGM_URL=" + c.opt.URL,
	}

	if c.opt.UseKeytab {
		cmd.Env = append(cmd.Env, "XrdSecPROTOCOL=sss")
		cmd.Env = append(cmd.Env, "XrdSecSSSKT="+c.opt.Keytab)
	} else { // we are a trusted gateway
		cmd.Env = append(cmd.Env, "XrdSecPROTOCOL=unix")
		cmd.Env = append(cmd.Env, "KRB5CCNAME=FILE:/dev/null") // do not try to use krb
	}

	err := cmd.Run()

	var exitStatus int
	if exiterr, ok := err.(*exec.ExitError); ok {
		// The program has exited with an exit code != 0
		// This works on both Unix and Windows. Although package
		// syscall is generally platform dependent, WaitStatus is
		// defined for both Unix and Windows and in both cases has
		// an ExitStatus() method with the same signature.
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			exitStatus = status.ExitStatus()
			switch exitStatus {
			case 0:
				err = nil
			case int(syscall.ENOENT):
				err = errtypes.NotFound(errBuf.String())
			}
		}
	}

	// check for operation not permitted error
	if strings.Contains(errBuf.String(), "Operation not permitted") {
		err = errtypes.InvalidCredentials("eosclient: no sufficient permissions for the operation")
	}

	// check for lock mismatch error
	if strings.Contains(errBuf.String(), "file has a valid extended attribute lock") {
		err = errtypes.Conflict("eosclient: lock mismatch")
	}

	args := fmt.Sprintf("%s", cmd.Args)
	env := fmt.Sprintf("%s", cmd.Env)
	log.Info().Str("args", args).Str("env", env).Int("exit", exitStatus).Msg("eos cmd")

	return outBuf.String(), errBuf.String(), err
}

// exec executes only EOS commands the command and returns the stdout, stderr and return code.
func (c *Client) executeEOS(ctx context.Context, cmdArgs []string, auth eosclient.Authorization) (string, string, error) {
	log := appctx.GetLogger(ctx)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}

	cmd := exec.CommandContext(ctx, c.opt.EosBinary)
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	cmd.Env = []string{
		"EOS_MGM_URL=" + c.opt.URL,
	}

	if auth.Token != "" {
		cmd.Env = append(cmd.Env, "EOSAUTHZ="+auth.Token)
	} else if auth.Role.UID != "" && auth.Role.GID != "" {
		cmd.Args = append(cmd.Args, []string{"-r", auth.Role.UID, auth.Role.GID}...)
	}

	if c.opt.UseKeytab {
		cmd.Env = append(cmd.Env, "XrdSecPROTOCOL=sss")
		cmd.Env = append(cmd.Env, "XrdSecSSSKT="+c.opt.Keytab)
	} else { // we are a trusted gateway
		cmd.Env = append(cmd.Env, "XrdSecPROTOCOL=unix")
		cmd.Env = append(cmd.Env, "KRB5CCNAME=FILE:/dev/null") // do not try to use krb
	}

	// add application label
	// cmd.Args = append(cmd.Args, "-a", "reva_eosclient::meta")

	cmd.Args = append(cmd.Args, cmdArgs...)

	t := trace.Get(ctx)
	if t != "" {
		cmd.Args = append(cmd.Args, "--comment", t)
	}

	err := cmd.Run()

	var exitStatus int
	if exiterr, ok := err.(*exec.ExitError); ok {
		// The program has exited with an exit code != 0
		// This works on both Unix and Windows. Although package
		// syscall is generally platform dependent, WaitStatus is
		// defined for both Unix and Windows and in both cases has
		// an ExitStatus() method with the same signature.
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			exitStatus = status.ExitStatus()
			switch exitStatus {
			case 0:
				err = nil
			case int(syscall.ENOENT):
				err = errtypes.NotFound("eosclient: " + errBuf.String())
			case int(syscall.EPERM), int(syscall.E2BIG), int(syscall.EINVAL):
				// eos reports back error code 1 (EPERM) as a PermissionDenied error
				// eos reports back error code 7 (E2BIG) when the user is not allowed to read the directory
				// eos reports back error code 22 (EINVAL) when the user is not allowed to enter the instance
				errString := errBuf.String()
				if errString == "" {
					errString = fmt.Sprintf("rc = %d", exitStatus)
				}
				err = errtypes.PermissionDenied("eosclient: " + errString)
			default:
				err = errors.Wrap(err, fmt.Sprintf("eosclient: error while executing command: %s", errBuf.String()))
			}
		}
	}

	args := fmt.Sprintf("%s", cmd.Args)
	env := fmt.Sprintf("%s", cmd.Env)
	log.Info().Str("args", args).Str("env", env).Int("exit", exitStatus).Str("err", errBuf.String()).Msg("eos cmd")
	return outBuf.String(), errBuf.String(), err
}

// AddACL adds an new acl to EOS with the given aclType.
func (c *Client) AddACL(ctx context.Context, auth, rootAuth eosclient.Authorization, path string, pos uint, a *acl.Entry) error {
	finfo, err := c.getRawFileInfoByPath(ctx, auth, path)
	if err != nil {
		return err
	}

	sysACL := a.CitrineSerialize()
	args := []string{"acl", "--sys"}
	if finfo.IsDir {
		args = append(args, "--recursive")
	}

	// set position of ACLs to add. The default is to append to the end, so no arguments will be added in this case
	// the first position starts at 1 = eosclient.StartPosition
	if pos != eosclient.EndPosition {
		args = append(args, "--position", fmt.Sprint(pos))
	}

	args = append(args, sysACL, path)

	_, _, err = c.executeEOS(ctx, args, rootAuth)
	return err
}

// RemoveACL removes the acl from EOS.
func (c *Client) RemoveACL(ctx context.Context, auth, rootAuth eosclient.Authorization, path string, a *acl.Entry) error {
	finfo, err := c.getRawFileInfoByPath(ctx, auth, path)
	if err != nil {
		return err
	}

	a.Permissions = ""
	sysACL := a.CitrineSerialize()
	args := []string{"acl", "--sys"}
	if finfo.IsDir {
		args = append(args, "--recursive")
	}
	args = append(args, sysACL, path)

	_, _, err = c.executeEOS(ctx, args, rootAuth)
	return err
}

// UpdateACL updates the EOS acl.
func (c *Client) UpdateACL(ctx context.Context, auth, rootAuth eosclient.Authorization, path string, position uint, a *acl.Entry) error {
	return c.AddACL(ctx, auth, rootAuth, path, position, a)
}

// GetACL for a file.
func (c *Client) GetACL(ctx context.Context, auth eosclient.Authorization, path, aclType, target string) (*acl.Entry, error) {
	acls, err := c.ListACLs(ctx, auth, path)
	if err != nil {
		return nil, err
	}
	for _, a := range acls {
		if a.Type == aclType && a.Qualifier == target {
			return a, nil
		}
	}
	return nil, errtypes.NotFound(fmt.Sprintf("%s:%s", aclType, target))
}

// ListACLs returns the list of ACLs present under the given path.
// EOS returns uids/gid for Citrine version and usernames for older versions.
// For Citire we need to convert back the uid back to username.
func (c *Client) ListACLs(ctx context.Context, auth eosclient.Authorization, path string) ([]*acl.Entry, error) {
	parsedACLs, err := c.getACLForPath(ctx, auth, path)
	if err != nil {
		return nil, err
	}

	// EOS Citrine ACLs are stored with uid. The UID will be resolved to the
	// user opaque ID at the eosfs level.
	return parsedACLs.Entries, nil
}

func (c *Client) getACLForPath(ctx context.Context, auth eosclient.Authorization, path string) (*acl.ACLs, error) {
	finfo, err := c.GetFileInfoByPath(ctx, auth, path)
	if err != nil {
		return nil, err
	}

	return finfo.SysACL, nil
}

// GetFileInfoByInode returns the FileInfo by the given inode.
func (c *Client) GetFileInfoByInode(ctx context.Context, auth eosclient.Authorization, inode uint64) (*eosclient.FileInfo, error) {
	args := []string{"file", "info", fmt.Sprintf("inode:%d", inode), "-m"}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, err
	}
	info, err := c.parseFileInfo(ctx, stdout)
	if err != nil {
		return nil, err
	}

	if c.opt.VersionInvariant && eosclient.IsVersionFolder(info.File) {
		info, err = c.getFileInfoFromVersion(ctx, auth, info.File)
		if err != nil {
			return nil, err
		}
		info.Inode = inode
	}

	return c.mergeACLsAndAttrsForFiles(ctx, auth, info), nil
}

// GetFileInfoByFXID returns the FileInfo by the given file id in hexadecimal.
func (c *Client) GetFileInfoByFXID(ctx context.Context, auth eosclient.Authorization, fxid string) (*eosclient.FileInfo, error) {
	args := []string{"file", "info", fmt.Sprintf("fxid:%s", fxid), "-m"}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, err
	}

	info, err := c.parseFileInfo(ctx, stdout)
	if err != nil {
		return nil, err
	}

	return c.mergeACLsAndAttrsForFiles(ctx, auth, info), nil
}

// GetFileInfoByPath returns the FilInfo at the given path.
func (c *Client) GetFileInfoByPath(ctx context.Context, auth eosclient.Authorization, path string) (*eosclient.FileInfo, error) {
	args := []string{"file", "info", path, "-m"}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, err
	}
	info, err := c.parseFileInfo(ctx, stdout)
	if err != nil {
		return nil, err
	}

	if c.opt.VersionInvariant && !eosclient.IsVersionFolder(path) && !info.IsDir {
		ownerAuth := eosclient.Authorization{Role: eosclient.Role{
			UID: strconv.FormatUint(info.UID, 10),
			GID: strconv.FormatUint(info.GID, 10),
		}}
		if inode, err := c.getVersionFolderInode(ctx, auth, ownerAuth, path); err == nil {
			info.Inode = inode
		}
	}

	return c.mergeACLsAndAttrsForFiles(ctx, auth, info), nil
}

func (c *Client) getRawFileInfoByPath(ctx context.Context, auth eosclient.Authorization, path string) (*eosclient.FileInfo, error) {
	args := []string{"file", "info", path, "-m"}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, err
	}
	return c.parseFileInfo(ctx, stdout)
}

func (c *Client) mergeACLsAndAttrsForFiles(ctx context.Context, auth eosclient.Authorization, info *eosclient.FileInfo) *eosclient.FileInfo {
	// We need to inherit the ACLs for the parent directory as these are not available for files
	if !info.IsDir {
		parentInfo, err := c.getRawFileInfoByPath(ctx, auth, path.Dir(info.File))
		// Even if this call fails, at least return the current file object
		if err == nil {
			info.SysACL.Entries = append(info.SysACL.Entries, parentInfo.SysACL.Entries...)
		}
	}

	return info
}

// SetAttr sets an extended attributes on a path.
func (c *Client) SetAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, errorIfExists, recursive bool, path, app string) error {
	if !isValidAttribute(attr) {
		return errors.New("eos: attr is invalid: " + serializeAttribute(attr))
	}

	// Favorites need to be stored per user so handle these separately
	if attr.Type == eosclient.UserAttr && attr.Key == eosclient.FavoritesKey {
		info, err := c.getRawFileInfoByPath(ctx, auth, path)
		if err != nil {
			return err
		}
		return c.handleFavAttr(ctx, auth, attr, recursive, path, info, true)
	}
	return c.setEOSAttr(ctx, auth, attr, errorIfExists, recursive, path, app)
}

func (c *Client) setEOSAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, errorIfExists, recursive bool, path, app string) error {
	args := []string{}
	if app != "" {
		args = append(args, "-a", app)
	}
	args = append(args, "attr")
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, "set")
	if errorIfExists {
		args = append(args, "-c")
	}
	args = append(args, serializeAttribute(attr), path)

	_, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		var exErr *exec.ExitError
		if errors.As(err, &exErr) && exErr.ExitCode() == 17 { // EEXIST
			return eosclient.AttrAlreadyExistsError
		}
		if errors.As(err, &exErr) && exErr.ExitCode() == 16 { // EBUSY -> Locked
			return eosclient.FileIsLockedError
		}
		return err
	}
	return nil
}

func (c *Client) handleFavAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, recursive bool, path string, info *eosclient.FileInfo, set bool) error {
	var err error
	u := appctx.ContextMustGetUser(ctx)
	if info == nil {
		info, err = c.getRawFileInfoByPath(ctx, auth, path)
		if err != nil {
			return err
		}
	}
	favStr := info.Attrs[eosclient.FavoritesKey]
	favs, err := acl.Parse(favStr, acl.ShortTextForm)
	if err != nil {
		return err
	}
	if set {
		err = favs.SetEntry(acl.TypeUser, u.Id.OpaqueId, "1")
		if err != nil {
			return err
		}
	} else {
		favs.DeleteEntry(acl.TypeUser, u.Id.OpaqueId)
	}
	attr.Val = favs.Serialize()

	if attr.Val == "" {
		return c.unsetEOSAttr(ctx, auth, attr, recursive, path, "", true)
	} else {
		return c.setEOSAttr(ctx, auth, attr, false, recursive, path, "")
	}
}

// UnsetAttr unsets an extended attribute on a path.
func (c *Client) UnsetAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, recursive bool, path, app string) error {
	// In the case of handleFavs, we call unsetEOSAttr with deleteFavs = true, which is why this simply calls a subroutine
	return c.unsetEOSAttr(ctx, auth, attr, recursive, path, app, false)
}

// UnsetAttr unsets an extended attribute on a path.
func (c *Client) unsetEOSAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, recursive bool, path, app string, deleteFavs bool) error {
	if !isValidAttribute(attr) {
		return errors.New("eos: attr is invalid: " + serializeAttribute(attr))
	}

	var err error
	// Favorites need to be stored per user so handle these separately
	if !deleteFavs && attr.Type == eosclient.UserAttr && attr.Key == eosclient.FavoritesKey {
		info, err := c.getRawFileInfoByPath(ctx, auth, path)
		if err != nil {
			return err
		}
		return c.handleFavAttr(ctx, auth, attr, recursive, path, info, false)
	}

	var args []string
	if app != "" {
		args = append(args, "-a", app)
	}
	args = append(args, "attr")
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, "rm", fmt.Sprintf("%s.%s", attrTypeToString(attr.Type), attr.Key), path)

	_, _, err = c.executeEOS(ctx, args, auth)
	if err != nil {
		var exErr *exec.ExitError
		if errors.As(err, &exErr) && exErr.ExitCode() == 61 {
			return eosclient.AttrNotExistsError
		}
		return err
	}
	return nil
}

// GetAttr returns the attribute specified by key.
func (c *Client) GetAttr(ctx context.Context, auth eosclient.Authorization, key, path string) (*eosclient.Attribute, error) {
	args := []string{"attr", "get", key, path}
	attrOut, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, err
	}

	attr, err := deserializeAttribute(attrOut)
	if err != nil {
		return nil, err
	}
	return attr, nil
}

// GetAttrs returns all the attributes of a resource.
func (c *Client) GetAttrs(ctx context.Context, auth eosclient.Authorization, path string) ([]*eosclient.Attribute, error) {
	args := []string{"attr", "ls", path}
	attrOut, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, err
	}

	attrsStr := strings.Split(attrOut, "\n")
	attrs := make([]*eosclient.Attribute, 0, len(attrsStr))
	for _, line := range attrsStr {
		attr, err := deserializeAttribute(line)
		if err != nil {
			return nil, err
		}
		attrs = append(attrs, attr)
	}
	return attrs, nil
}

func deserializeAttribute(attrStr string) (*eosclient.Attribute, error) {
	// the string is in the form sys.forced.checksum="adler"
	keyValue := strings.SplitN(strings.TrimSpace(attrStr), "=", 2) // keyValue = ["sys.forced.checksum", "\"adler\""]
	if len(keyValue) != 2 {
		return nil, errtypes.InternalError("wrong attr format to deserialize")
	}
	type2key := strings.SplitN(keyValue[0], ".", 2) // type2key = ["sys", "forced.checksum"]
	if len(type2key) != 2 {
		return nil, errtypes.InternalError("wrong attr format to deserialize")
	}
	t, err := eosclient.AttrStringToType(type2key[0])
	if err != nil {
		return nil, err
	}
	// trim \" from value
	value := strings.Trim(keyValue[1], "\"")
	return &eosclient.Attribute{Type: t, Key: type2key[1], Val: value}, nil
}

// GetQuota gets the quota of a user on the quota node defined by path.
func (c *Client) GetQuota(ctx context.Context, username string, rootAuth eosclient.Authorization, path string) (*eosclient.QuotaInfo, error) {
	args := []string{"quota", "ls", "-u", username, "-m"}
	stdout, _, err := c.executeEOS(ctx, args, rootAuth)
	if err != nil {
		return nil, err
	}
	return c.parseQuota(path, stdout)
}

// SetQuota sets the quota of a user on the quota node defined by path.
func (c *Client) SetQuota(ctx context.Context, rootAuth eosclient.Authorization, info *eosclient.SetQuotaInfo) error {
	maxBytes := fmt.Sprintf("%d", info.MaxBytes)
	maxFiles := fmt.Sprintf("%d", info.MaxFiles)
	args := []string{"quota", "set", "-u", info.Username, "-p", info.QuotaNode, "-v", maxBytes, "-i", maxFiles}
	_, _, err := c.executeEOS(ctx, args, rootAuth)
	if err != nil {
		return err
	}
	return nil
}

// Touch creates a 0-size,0-replica file in the EOS namespace.
func (c *Client) Touch(ctx context.Context, auth eosclient.Authorization, path string) error {
	args := []string{"file", "touch", path}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// Chown given path.
func (c *Client) Chown(ctx context.Context, auth, chownauth eosclient.Authorization, path string) error {
	args := []string{"chown", chownauth.Role.UID + ":" + chownauth.Role.GID, path}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// Chmod given path.
func (c *Client) Chmod(ctx context.Context, auth eosclient.Authorization, mode, path string) error {
	args := []string{"chmod", mode, path}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// CreateDir creates a directory at the given path.
func (c *Client) CreateDir(ctx context.Context, auth eosclient.Authorization, path string) error {
	args := []string{"mkdir", "-p", path}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// Remove removes the resource at the given path.
func (c *Client) Remove(ctx context.Context, auth eosclient.Authorization, path string, noRecycle bool) error {
	args := []string{"rm", "-r"}
	if noRecycle {
		args = append(args, "--no-recycle-bin") // do not put the file in the recycle bin
	}
	args = append(args, path)
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// Rename renames the resource referenced by oldPath to newPath.
func (c *Client) Rename(ctx context.Context, auth eosclient.Authorization, oldPath, newPath string) error {
	args := []string{"file", "rename", oldPath, newPath}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

func (c *Client) ListWithRegex(ctx context.Context, auth eosclient.Authorization, path string, depth uint, regex string) ([]*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("regex", regex).Uint("depth", depth).Msg("ListWithRegex")
	// here we want to use --skip-version-dirs and drop -f so to have version folders' metadata in the results without access errors, but need EOS 5.3 for that. So for now we restrict to files and use a cache afterwards...
	args := []string{"newfind", "--fileinfo", "--maxdepth", strconv.Itoa(int(depth)), "--name", regex, "-f", path}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, errors.Wrapf(err, "eosclient: error listing fn=%s", path)
	}
	return c.parseFind(ctx, auth, path, stdout, true)
}

// List the contents of the directory given by path.
func (c *Client) List(ctx context.Context, auth eosclient.Authorization, path string) ([]*eosclient.FileInfo, error) {
	args := []string{"newfind", "--fileinfo", "--skip-version-dirs", "--maxdepth", "1", path}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, errors.Wrapf(err, "eosclient: error listing fn=%s", path)
	}
	return c.parseFind(ctx, auth, path, stdout, false)
}

// Read reads a file from the mgm.
func (c *Client) Read(ctx context.Context, auth eosclient.Authorization, path string) (io.ReadCloser, error) {
	rand := "eosread-" + uuid.New().String()
	localTarget := fmt.Sprintf("%s/%s", c.opt.CacheDirectory, rand)
	defer os.RemoveAll(localTarget)

	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	args := []string{"--nopbar", "--silent", "-f", xrdPath, localTarget}

	if auth.Token != "" {
		args[3] += "?authz=" + auth.Token
	} else if auth.Role.UID != "" && auth.Role.GID != "" {
		args = append(args, fmt.Sprintf("-OSeos.ruid=%s&eos.rgid=%s&eos.app=reva_eosclient::read", auth.Role.UID, auth.Role.GID))
	}

	_, _, err := c.executeXRDCopy(ctx, args)
	if err != nil {
		return nil, err
	}
	return os.Open(localTarget)
}

// Write writes a stream to the mgm.
func (c *Client) Write(ctx context.Context, auth eosclient.Authorization, path string, stream io.ReadCloser, length int64, app string, disableVersioning bool) error {
	fd, err := os.CreateTemp(c.opt.CacheDirectory, "eoswrite-")
	if err != nil {
		return err
	}
	defer fd.Close()
	defer os.RemoveAll(fd.Name())

	// copy stream to local temp file
	_, err = io.Copy(fd, stream)
	if err != nil {
		return err
	}
	return c.writeFile(ctx, auth, path, fd.Name(), app, disableVersioning)
}

// WriteFile writes an existing file to the mgm.
func (c *Client) writeFile(ctx context.Context, auth eosclient.Authorization, path, source, app string, disableVersioning bool) error {
	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	args := []string{"--nopbar", "--silent", "-f", source, xrdPath}

	options := fmt.Sprintf("-ODeos.app=%s", app)
	if disableVersioning {
		options += "&eos.versioning=0"
	}

	if auth.Token != "" {
		args[4] += "?authz=" + auth.Token
	} else if auth.Role.UID != "" && auth.Role.GID != "" {
		options += fmt.Sprintf("&eos.ruid=%s&eos.rgid=%s", auth.Role.UID, auth.Role.GID)
	} else {
		return errors.New("No authentication provided")
	}
	args = append(args, options)

	_, _, err := c.executeXRDCopy(ctx, args)
	return err
}

// ListDeletedEntries returns a list of the deleted entries.
func (c *Client) ListDeletedEntries(ctx context.Context, auth eosclient.Authorization, maxentries int, from, to time.Time) ([]*eosclient.DeletedEntry, error) {
	deleted := []*eosclient.DeletedEntry{}
	count := 0
	for d := to; !d.Before(from); d = d.AddDate(0, 0, -1) {
		args := []string{"recycle", "ls", "-m", d.Format("2006/01/02"), fmt.Sprintf("%d", maxentries+1)}
		stdout, _, err := c.executeEOS(ctx, args, auth)
		if err != nil {
			switch err.(type) {
			case errtypes.IsPermissionDenied:
				// in this context, this is an E2BIG that gets converted to PermissionDenied by executeEOS()
				return nil, errtypes.BadRequest("list too long")
			default:
				return nil, err
			}
		}

		list, err := parseRecycleList(stdout)
		if err != nil {
			return nil, err
		}
		deleted = append(deleted, list...)
		count += len(list)
		if count > maxentries {
			return nil, errtypes.BadRequest("list too long")
		}
	}

	return deleted, nil
}

// RestoreDeletedEntry restores a deleted entry.
func (c *Client) RestoreDeletedEntry(ctx context.Context, auth eosclient.Authorization, key string) error {
	args := []string{"recycle", "restore", key}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// PurgeDeletedEntries purges all entries from the recycle bin.
func (c *Client) PurgeDeletedEntries(ctx context.Context, auth eosclient.Authorization) error {
	args := []string{"recycle", "purge"}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// ListVersions list all the versions for a given file.
func (c *Client) ListVersions(ctx context.Context, auth eosclient.Authorization, p string) ([]*eosclient.FileInfo, error) {
	versionFolder := eosclient.GetVersionFolder(p)
	finfos, err := c.List(ctx, auth, versionFolder)
	if err != nil {
		// we send back an empty list
		return []*eosclient.FileInfo{}, nil
	}
	return finfos, nil
}

// RollbackToVersion rollbacks a file to a previous version.
func (c *Client) RollbackToVersion(ctx context.Context, auth eosclient.Authorization, path, version string) error {
	args := []string{"file", "versions", path, version}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// ReadVersion reads the version for the given file.
func (c *Client) ReadVersion(ctx context.Context, auth eosclient.Authorization, p, version string) (io.ReadCloser, error) {
	versionFile := path.Join(eosclient.GetVersionFolder(p), version)
	return c.Read(ctx, auth, versionFile)
}

// GenerateToken returns a token on behalf of the resource owner to be used by lightweight accounts.
func (c *Client) GenerateToken(ctx context.Context, auth eosclient.Authorization, p string, a *acl.Entry) (string, error) {
	expiration := strconv.FormatInt(time.Now().Add(time.Duration(c.opt.TokenExpiry)*time.Second).Unix(), 10)
	args := []string{"token", "--permission", a.Permissions, "--tree", "--path", p, "--expires", expiration}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	return strings.TrimSpace(stdout), err
}

func (c *Client) getVersionFolderInode(ctx context.Context, auth, ownerAuth eosclient.Authorization, p string) (uint64, error) {
	versionFolder := eosclient.GetVersionFolder(p)
	md, err := c.getRawFileInfoByPath(ctx, auth, versionFolder)
	if err != nil {
		if err = c.CreateDir(ctx, ownerAuth, versionFolder); err != nil {
			return 0, err
		}
		md, err = c.getRawFileInfoByPath(ctx, auth, versionFolder)
		if err != nil {
			return 0, err
		}
	}
	return md.Inode, nil
}

func (c *Client) getFileInfoFromVersion(ctx context.Context, auth eosclient.Authorization, p string) (*eosclient.FileInfo, error) {
	file := eosclient.GetFileFromVersionFolder(p)
	md, err := c.GetFileInfoByPath(ctx, auth, file)
	if err != nil {
		return nil, err
	}
	return md, nil
}

func parseRecycleList(raw string) ([]*eosclient.DeletedEntry, error) {
	entries := []*eosclient.DeletedEntry{}
	rawLines := strings.FieldsFunc(raw, func(c rune) bool {
		return c == '\n'
	})
	for _, rl := range rawLines {
		if rl == "" {
			continue
		}
		entry, err := parseRecycleEntry(rl)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// parse entries like these:
// recycle=ls recycle-bin=/eos/backup/proc/recycle/ uid=gonzalhu gid=it size=0 deletion-time=1510823151 type=recursive-dir keylength.restore-path=45 restore-path=/eos/scratch/user/g/gonzalhu/.sys.v#.app.ico/ restore-key=0000000000a35100
// recycle=ls recycle-bin=/eos/backup/proc/recycle/ uid=gonzalhu gid=it size=381038 deletion-time=1510823151 type=file keylength.restore-path=36 restore-path=/eos/scratch/user/g/gonzalhu/app.ico restore-key=000000002544fdb3.
// NOTE: after EOS 5.2.0, the restore-key field is not the latest entry in the response anymore.
func parseRecycleEntry(raw string) (*eosclient.DeletedEntry, error) {
	partsBySpace := strings.FieldsFunc(raw, func(c rune) bool {
		return c == ' '
	})

	kv := getMap(partsBySpace)
	size, err := strconv.ParseUint(kv["size"], 10, 64)
	if err != nil {
		return nil, err
	}
	isDir := false
	if kv["type"] == "recursive-dir" {
		isDir = true
	}
	deletionMTime, err := strconv.ParseUint(strings.Split(kv["deletion-time"], ".")[0], 10, 64)
	if err != nil {
		return nil, err
	}
	entry := &eosclient.DeletedEntry{
		RestorePath:   kv["restore-path"],
		RestoreKey:    kv["restore-key"],
		Size:          size,
		DeletionMTime: deletionMTime,
		IsDir:         isDir,
	}

	// rewrite the restore-path to take into account the key keylength.restore-path
	keyLengthString, ok := kv["keylength.restore-path"]
	if !ok {
		return nil, errors.Wrap(err, fmt.Sprintf("eos response is missing restore-key:%+v", kv))
	}

	keyLength, err := strconv.ParseUint(keyLengthString, 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("recycle ls response keylength.restore-path is not a number:%+v", kv))
	}

	// find the index of the restore-path key string in the raw string
	// ... restore-path=/eos/scratch/user/g/gonzalhu/app.ico ....
	// NOTE: this code will break if another key of the output will contain the string "restore-path=/" in it (very unlikely)
	index := strings.Index(raw, "restore-path=/")
	if index == -1 {
		return nil, errors.New(fmt.Sprintf("restore-path key not found in raw string: %s", raw))
	}
	start := index + len("restore-path=/") // note the key ends with /, this is to avoid getting a hit on keylength.restore-path
	stop := uint64(start) + keyLength
	restorePath := raw[start:stop]
	restorePath = "/" + restorePath // if the path does not start with /, it's skipping in response
	restorePath = strings.Trim(restorePath, " ")

	entry.RestorePath = restorePath

	return entry, nil
}

func getMap(partsBySpace []string) map[string]string {
	kv := map[string]string{}
	for _, pair := range partsBySpace {
		parts := strings.Split(pair, "=")
		if len(parts) > 1 {
			kv[parts[0]] = parts[1]
		}
	}
	return kv
}

func (c *Client) parseFind(ctx context.Context, auth eosclient.Authorization, dirPath, raw string, cache bool) ([]*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)

	finfos := []*eosclient.FileInfo{}
	versionFolders := map[string]*eosclient.FileInfo{}
	rawLines := strings.FieldsFunc(raw, func(c rune) bool {
		return c == '\n'
	})

	var ownerAuth *eosclient.Authorization

	var parent *eosclient.FileInfo
	for _, rl := range rawLines {
		if rl == "" {
			continue
		}
		fi, err := c.parseFileInfo(ctx, rl)
		if err != nil {
			return nil, err
		}
		// dirs in eos end with a slash, like /eos/user/g/gonzalhu/
		// we skip the current directory as eos find will return the directory we
		// ask to find
		if fi.File == path.Clean(dirPath) {
			parent = fi
			continue
		}

		// If it's a version folder, store it in a map, so that for the corresponding file,
		// we can return its inode instead
		if eosclient.IsVersionFolder(fi.File) {
			versionFolders[fi.File] = fi
		}

		if ownerAuth == nil {
			ownerAuth = &eosclient.Authorization{
				Role: eosclient.Role{
					UID: strconv.FormatUint(fi.UID, 10),
					GID: strconv.FormatUint(fi.GID, 10),
				},
			}
		}

		finfos = append(finfos, fi)
	}

	for _, fi := range finfos {
		// For files, inherit ACLs from the parent
		// And set the inode to that of their version folder
		if !fi.IsDir && !eosclient.IsVersionFolder(dirPath) {
			if parent != nil {
				fi.SysACL.Entries = append(fi.SysACL.Entries, parent.SysACL.Entries...)
			}
			versionFolderPath := eosclient.GetVersionFolder(fi.File)
			vf, ok := versionFolders[versionFolderPath]
			if cache && !ok {
				verfolder, err := c.versionFolderCache.Get(versionFolderPath)
				if err == nil && verfolder != nil {
					vf, ok = verfolder.(*eosclient.FileInfo)
				}
			}
			if ok {
				fi.Inode = vf.Inode
				fi.SysACL.Entries = append(fi.SysACL.Entries, vf.SysACL.Entries...)
				for k, v := range vf.Attrs {
					fi.Attrs[k] = v
				}
			} else if err := c.CreateDir(ctx, *ownerAuth, versionFolderPath); err == nil { // Create the version folder if it doesn't exist
				if md, err := c.getRawFileInfoByPath(ctx, auth, versionFolderPath); err == nil {
					fi.Inode = md.Inode
				} else {
					log.Error().Err(err).Interface("auth", ownerAuth).Str("path", versionFolderPath).Msg("got error creating version folder")
				}
			}
			if cache {
				c.versionFolderCache.Set(versionFolderPath, fi)
			}
		}
	}

	return finfos, nil
}

func (c Client) parseEosOutputLine(line string) map[string]string {
	partsBySpace := strings.FieldsFunc(line, func(c rune) bool {
		return c == ' '
	})
	m := getMap(partsBySpace)
	return m
}

func (c *Client) parseQuota(path, raw string) (*eosclient.QuotaInfo, error) {
	rawLines := strings.FieldsFunc(raw, func(c rune) bool {
		return c == '\n'
	})
	for _, rl := range rawLines {
		if rl == "" {
			continue
		}

		m := c.parseEosOutputLine(rl)
		// map[maxbytes:2000000000000 maxlogicalbytes:1000000000000 percentageusedbytes:0.49 quota:node uid:gonzalhu space:/eos/scratch/user/ usedbytes:9829986500 usedlogicalbytes:4914993250 statusfiles:ok usedfiles:334 maxfiles:1000000 statusbytes:ok]

		space := m["space"]
		if strings.HasPrefix(path, filepath.Clean(space)) {
			maxBytesString := m["maxlogicalbytes"]
			usedBytesString := m["usedlogicalbytes"]
			maxBytes, _ := strconv.ParseUint(maxBytesString, 10, 64)
			usedBytes, _ := strconv.ParseUint(usedBytesString, 10, 64)

			maxInodesString := m["maxfiles"]
			usedInodesString := m["usedfiles"]
			maxInodes, _ := strconv.ParseUint(maxInodesString, 10, 64)
			usedInodes, _ := strconv.ParseUint(usedInodesString, 10, 64)

			qi := &eosclient.QuotaInfo{
				TotalBytes:  maxBytes,
				UsedBytes:   usedBytes,
				TotalInodes: maxInodes,
				UsedInodes:  usedInodes,
			}
			return qi, nil
		}
	}
	return &eosclient.QuotaInfo{}, nil
}

// TODO(labkode): better API to access extended attributes.
func (c *Client) parseFileInfo(ctx context.Context, raw string) (*eosclient.FileInfo, error) {
	line := raw[15:]
	index := strings.Index(line, " file=/")
	lengthString := line[0:index]
	length, err := strconv.ParseUint(lengthString, 10, 64)
	if err != nil {
		return nil, err
	}

	line = line[index+6:] // skip ' file='
	name := line[0:length]

	kv := make(map[string]string)
	attrs := make(map[string]string)
	// strip trailing slash
	kv["file"] = strings.TrimSuffix(name, "/")

	line = line[length+1:]
	partsBySpace := strings.FieldsFunc(line, func(c rune) bool { // we have [size=45 container=3 ...}
		return c == ' '
	})
	previousXAttr := ""
	for _, p := range partsBySpace {
		partsByEqual := strings.SplitN(p, "=", 2) // we have kv pairs like [size 14]
		if len(partsByEqual) == 2 {
			// handle xattrn and xattrv special cases
			switch {
			case partsByEqual[0] == "xattrn":
				previousXAttr = partsByEqual[1]
				if previousXAttr != "user.acl" {
					previousXAttr = strings.Replace(previousXAttr, "user.", "", 1)
				}
			case partsByEqual[0] == "xattrv":
				attrs[previousXAttr] = strings.ToValidUTF8(partsByEqual[1], "")
				previousXAttr = ""
			default:
				kv[partsByEqual[0]] = partsByEqual[1]
			}
		}
	}
	fi, err := c.mapToFileInfo(ctx, kv, attrs)
	if err != nil {
		return nil, err
	}
	return fi, nil
}

// mapToFileInfo converts the dictionary to an usable structure.
// The kv has format:
// map[sys.forced.space:default files:0 mode:42555 ino:5 sys.forced.blocksize:4k sys.forced.layout:replica uid:0 fid:5 sys.forced.blockchecksum:crc32c sys.recycle:/eos/backup/proc/recycle/ fxid:00000005 pid:1 etag:5:0.000 keylength.file:4 file:/eos treesize:1931593933849913 container:3 gid:0 mtime:1498571294.108614409 ctime:1460121992.294326762 pxid:00000001 sys.forced.checksum:adler sys.forced.nstripes:2].
func (c *Client) mapToFileInfo(ctx context.Context, kv, attrs map[string]string) (*eosclient.FileInfo, error) {
	inode, err := strconv.ParseUint(kv["ino"], 10, 64)
	if err != nil {
		return nil, err
	}
	fid, err := strconv.ParseUint(kv["fid"], 10, 64)
	if err != nil {
		return nil, err
	}
	uid, err := strconv.ParseUint(kv["uid"], 10, 64)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(kv["gid"], 10, 64)
	if err != nil {
		return nil, err
	}

	var treeSize uint64
	// treeSize is only for containers, so we check
	if val, ok := kv["treesize"]; ok {
		treeSize, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	var fileCounter uint64
	// fileCounter is only for containers
	if val, ok := kv["files"]; ok {
		fileCounter, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	var dirCounter uint64
	// dirCounter is only for containers
	if val, ok := kv["container"]; ok {
		dirCounter, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return nil, err
		}
	}

	// treeCount is the number of entries under the tree
	treeCount := fileCounter + dirCounter

	var size uint64
	if val, ok := kv["size"]; ok {
		size, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return nil, err
		}
	}

	// look for the stime first as mtime is not updated for parent dirs; if that isn't set, we use mtime
	var mtimesec, mtimenanos uint64
	var mtimeSet bool
	if val, ok := kv["stime"]; ok && val != "" {
		stimeSplit := strings.Split(val, ".")
		if mtimesec, err = strconv.ParseUint(stimeSplit[0], 10, 64); err == nil {
			mtimeSet = true
		}
		if mtimenanos, err = strconv.ParseUint(stimeSplit[1], 10, 32); err != nil {
			mtimeSet = false
		}
	}
	if !mtimeSet {
		mtimeSplit := strings.Split(kv["mtime"], ".")
		mtimesec, _ = strconv.ParseUint(mtimeSplit[0], 10, 64)
		mtimenanos, _ = strconv.ParseUint(mtimeSplit[1], 10, 32)
	}

	var ctimesec, ctimenanos uint64
	if val, ok := kv["ctime"]; ok && val != "" {
		if split := strings.Split(val, "."); len(split) >= 2 {
			ctimesec, _ = strconv.ParseUint(split[0], 10, 64)
			ctimenanos, _ = strconv.ParseUint(split[1], 10, 32)
		}
	}

	var atimesec, atimenanos uint64
	if val, ok := kv["atime"]; ok && val != "" {
		if split := strings.Split(val, "."); len(split) >= 2 {
			atimesec, _ = strconv.ParseUint(split[0], 10, 64)
			atimenanos, _ = strconv.ParseUint(split[1], 10, 32)
		}
	}

	isDir := false
	var xs *eosclient.Checksum
	if _, ok := kv["files"]; ok {
		isDir = true
	} else {
		xs = &eosclient.Checksum{
			XSSum:  kv["xs"],
			XSType: kv["xstype"],
		}
	}

	sysACL, err := acl.Parse(attrs["sys.acl"], acl.ShortTextForm)
	if err != nil {
		return nil, err
	}

	fi := &eosclient.FileInfo{
		File:       kv["file"],
		Inode:      inode,
		FID:        fid,
		UID:        uid,
		GID:        gid,
		ETag:       kv["etag"],
		Size:       size,
		TreeSize:   treeSize,
		MTimeSec:   mtimesec,
		MTimeNanos: uint32(mtimenanos),
		CTimeSec:   ctimesec,
		CTimeNanos: uint32(ctimenanos),
		ATimeSec:   atimesec,
		ATimeNanos: uint32(atimenanos),
		IsDir:      isDir,
		Instance:   c.opt.URL,
		SysACL:     sysACL,
		TreeCount:  treeCount,
		Attrs:      attrs,
		XS:         xs,
	}

	return fi, nil
}
