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

package eosbinary

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/eosclient"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/acl"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

const (
	versionPrefix  = ".sys.v#."
	lwShareAttrKey = "reva.lwshare"
)

const (
	// SystemAttr is the system extended attribute.
	SystemAttr eosclient.AttrType = iota
	// UserAttr is the user extended attribute.
	UserAttr
)

func serializeAttribute(a *eosclient.Attribute) string {
	return fmt.Sprintf("%s.%s=%s", attrTypeToString(a.Type), a.Key, a.Val)
}

func attrTypeToString(at eosclient.AttrType) string {
	switch at {
	case SystemAttr:
		return "sys"
	case UserAttr:
		return "user"
	default:
		return "invalid"
	}
}

func isValidAttribute(a *eosclient.Attribute) bool {
	// validate that an attribute is correct.
	if (a.Type != SystemAttr && a.Type != UserAttr) || a.Key == "" {
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
	SecProtocol string

	// TokenExpiry stores in seconds the time after which generated tokens will expire
	// Default is 3600
	TokenExpiry int
}

func (opt *Options) init() {
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
}

// New creates a new client with the given options.
func New(opt *Options) (*Client, error) {
	opt.init()
	c := new(Client)
	c.opt = opt
	return c, nil
}

// executeXRDCopy executes xrdcpy commands and returns the stdout, stderr and return code
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
		cmd.Env = append(cmd.Env, "XrdSecPROTOCOL="+c.opt.SecProtocol)
		cmd.Env = append(cmd.Env, "XrdSecSSSKT="+c.opt.Keytab)
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

	args := fmt.Sprintf("%s", cmd.Args)
	env := fmt.Sprintf("%s", cmd.Env)
	log.Info().Str("args", args).Str("env", env).Int("exit", exitStatus).Msg("eos cmd")

	if err != nil && exitStatus != int(syscall.ENOENT) { // don't wrap the errtypes.NotFoundError
		err = errors.Wrap(err, "eosclient: error while executing command")
	}

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
		cmd.Env = append(cmd.Env, "XrdSecPROTOCOL="+c.opt.SecProtocol)
		cmd.Env = append(cmd.Env, "XrdSecSSSKT="+c.opt.Keytab)
	}

	cmd.Args = append(cmd.Args, cmdArgs...)

	trace := trace.FromContext(ctx).SpanContext().TraceID.String()
	cmd.Args = append(cmd.Args, "--comment", trace)

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
			case int(syscall.EPERM), int(syscall.E2BIG), int(syscall.EINVAL):
				// eos reports back error code 1 (EPERM) when ?
				// eos reports back error code 7 (E2BIG) when the user is not allowed to read the directory
				// eos reports back error code 22 (EINVAL) when the user is not allowed to enter the instance
				err = errtypes.PermissionDenied(errBuf.String())
			}
		}
	}

	args := fmt.Sprintf("%s", cmd.Args)
	env := fmt.Sprintf("%s", cmd.Env)
	log.Info().Str("args", args).Str("env", env).Int("exit", exitStatus).Str("err", errBuf.String()).Msg("eos cmd")

	if err != nil && exitStatus != int(syscall.ENOENT) { // don't wrap the errtypes.NotFoundError
		err = errors.Wrap(err, "eosclient: error while executing command")
	}

	return outBuf.String(), errBuf.String(), err
}

// AddACL adds an new acl to EOS with the given aclType.
func (c *Client) AddACL(ctx context.Context, auth, rootAuth eosclient.Authorization, path string, a *acl.Entry) error {
	finfo, err := c.GetFileInfoByPath(ctx, auth, path)
	if err != nil {
		return err
	}

	if a.Type == acl.TypeLightweight {
		sysACL := ""
		aclStr, ok := finfo.Attrs[lwShareAttrKey]
		if ok {
			acls, err := acl.Parse(aclStr, acl.ShortTextForm)
			if err != nil {
				return err
			}
			err = acls.SetEntry(a.Type, a.Qualifier, a.Permissions)
			if err != nil {
				return err
			}
			sysACL = acls.Serialize()
		} else {
			sysACL = a.CitrineSerialize()
		}
		sysACLAttr := &eosclient.Attribute{
			Type: SystemAttr,
			Key:  lwShareAttrKey,
			Val:  sysACL,
		}
		if err = c.SetAttr(ctx, auth, sysACLAttr, true, path); err != nil {
			return err
		}
		return nil
	}

	sysACL := a.CitrineSerialize()
	args := []string{"acl"}

	if finfo.IsDir {
		args = append(args, "--sys", "--recursive")
	} else {
		args = append(args, "--user")
		userACLAttr := &eosclient.Attribute{
			Type: SystemAttr,
			Key:  "eval.useracl",
			Val:  "1",
		}
		if err = c.SetAttr(ctx, auth, userACLAttr, false, path); err != nil {
			return err
		}
	}
	args = append(args, sysACL, path)

	_, _, err = c.executeEOS(ctx, args, rootAuth)
	return err

}

// RemoveACL removes the acl from EOS.
func (c *Client) RemoveACL(ctx context.Context, auth, rootAuth eosclient.Authorization, path string, a *acl.Entry) error {
	finfo, err := c.GetFileInfoByPath(ctx, auth, path)
	if err != nil {
		return err
	}

	if a.Type == acl.TypeLightweight {
		sysACL := ""
		aclStr, ok := finfo.Attrs[lwShareAttrKey]
		if ok {
			acls, err := acl.Parse(aclStr, acl.ShortTextForm)
			if err != nil {
				return err
			}
			acls.DeleteEntry(a.Type, a.Qualifier)
			if err != nil {
				return err
			}
			sysACL = acls.Serialize()
		} else {
			sysACL = a.CitrineSerialize()
		}
		sysACLAttr := &eosclient.Attribute{
			Type: SystemAttr,
			Key:  lwShareAttrKey,
			Val:  sysACL,
		}
		if err = c.SetAttr(ctx, auth, sysACLAttr, true, path); err != nil {
			return err
		}
		return nil
	}

	sysACL := a.CitrineSerialize()
	args := []string{"acl"}
	if finfo.IsDir {
		args = append(args, "--sys", "--recursive")
	} else {
		args = append(args, "--user")
		userACLAttr := &eosclient.Attribute{
			Type: SystemAttr,
			Key:  "eval.useracl",
		}
		if err = c.UnsetAttr(ctx, auth, userACLAttr, path); err != nil {
			return err
		}
	}
	args = append(args, sysACL, path)

	_, _, err = c.executeEOS(ctx, args, rootAuth)
	return err
}

// UpdateACL updates the EOS acl.
func (c *Client) UpdateACL(ctx context.Context, auth, rootAuth eosclient.Authorization, path string, a *acl.Entry) error {
	return c.AddACL(ctx, auth, rootAuth, path, a)
}

// GetACL for a file
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

// GetFileInfoByInode returns the FileInfo by the given inode
func (c *Client) GetFileInfoByInode(ctx context.Context, auth eosclient.Authorization, inode uint64) (*eosclient.FileInfo, error) {
	args := []string{"file", "info", fmt.Sprintf("inode:%d", inode), "-m"}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, err
	}
	info, err := c.parseFileInfo(stdout)
	if err != nil {
		return nil, err
	}

	if c.opt.VersionInvariant && isVersionFolder(info.File) {
		info, err = c.getFileInfoFromVersion(ctx, auth, info.File)
		if err != nil {
			return nil, err
		}
		info.Inode = inode
	}

	return info, nil
}

// GetFileInfoByFXID returns the FileInfo by the given file id in hexadecimal
func (c *Client) GetFileInfoByFXID(ctx context.Context, auth eosclient.Authorization, fxid string) (*eosclient.FileInfo, error) {
	args := []string{"file", "info", fmt.Sprintf("fxid:%s", fxid), "-m"}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, err
	}
	return c.parseFileInfo(stdout)
}

// GetFileInfoByPath returns the FilInfo at the given path
func (c *Client) GetFileInfoByPath(ctx context.Context, auth eosclient.Authorization, path string) (*eosclient.FileInfo, error) {
	args := []string{"file", "info", path, "-m"}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, err
	}
	info, err := c.parseFileInfo(stdout)
	if err != nil {
		return nil, err
	}

	if c.opt.VersionInvariant && !isVersionFolder(path) && !info.IsDir {
		if inode, err := c.getVersionFolderInode(ctx, auth, path); err == nil {
			info.Inode = inode
		}
	}

	return info, nil
}

// SetAttr sets an extended attributes on a path.
func (c *Client) SetAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, recursive bool, path string) error {
	if !isValidAttribute(attr) {
		return errors.New("eos: attr is invalid: " + serializeAttribute(attr))
	}
	var args []string
	if recursive {
		args = []string{"attr", "-r", "set", serializeAttribute(attr), path}
	} else {
		args = []string{"attr", "set", serializeAttribute(attr), path}
	}

	_, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return err
	}
	return nil
}

// UnsetAttr unsets an extended attribute on a path.
func (c *Client) UnsetAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, path string) error {
	if !isValidAttribute(attr) {
		return errors.New("eos: attr is invalid: " + serializeAttribute(attr))
	}
	args := []string{"attr", "-r", "rm", fmt.Sprintf("%d.%s", attr.Type, attr.Key), path}
	_, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return err
	}
	return nil
}

// GetQuota gets the quota of a user on the quota node defined by path
func (c *Client) GetQuota(ctx context.Context, username string, rootAuth eosclient.Authorization, path string) (*eosclient.QuotaInfo, error) {
	args := []string{"quota", "ls", "-u", username, "-m"}
	stdout, _, err := c.executeEOS(ctx, args, rootAuth)
	if err != nil {
		return nil, err
	}
	return c.parseQuota(path, stdout)
}

// SetQuota sets the quota of a user on the quota node defined by path
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

// Chown given path
func (c *Client) Chown(ctx context.Context, auth, chownauth eosclient.Authorization, path string) error {
	args := []string{"chown", chownauth.Role.UID + ":" + chownauth.Role.GID, path}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// Chmod given path
func (c *Client) Chmod(ctx context.Context, auth eosclient.Authorization, mode, path string) error {
	args := []string{"chmod", mode, path}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// CreateDir creates a directory at the given path
func (c *Client) CreateDir(ctx context.Context, auth eosclient.Authorization, path string) error {
	args := []string{"mkdir", "-p", path}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// Remove removes the resource at the given path
func (c *Client) Remove(ctx context.Context, auth eosclient.Authorization, path string) error {
	args := []string{"rm", "-r", path}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// Rename renames the resource referenced by oldPath to newPath
func (c *Client) Rename(ctx context.Context, auth eosclient.Authorization, oldPath, newPath string) error {
	args := []string{"file", "rename", oldPath, newPath}
	_, _, err := c.executeEOS(ctx, args, auth)
	return err
}

// List the contents of the directory given by path
func (c *Client) List(ctx context.Context, auth eosclient.Authorization, path string) ([]*eosclient.FileInfo, error) {
	args := []string{"find", "--fileinfo", "--maxdepth", "1", path}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, errors.Wrapf(err, "eosclient: error listing fn=%s", path)
	}
	return c.parseFind(path, stdout)
}

// Read reads a file from the mgm
func (c *Client) Read(ctx context.Context, auth eosclient.Authorization, path string) (io.ReadCloser, error) {
	rand := "eosread-" + uuid.New().String()
	localTarget := fmt.Sprintf("%s/%s", c.opt.CacheDirectory, rand)
	defer os.RemoveAll(localTarget)

	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	args := []string{"--nopbar", "--silent", "-f", xrdPath, localTarget}

	if auth.Token != "" {
		args[3] += "?authz=" + auth.Token
	} else if auth.Role.UID != "" && auth.Role.GID != "" {
		args = append(args, fmt.Sprintf("-OSeos.ruid=%s&eos.rgid=%s", auth.Role.UID, auth.Role.GID))
	}

	_, _, err := c.executeXRDCopy(ctx, args)
	if err != nil {
		return nil, err
	}
	return os.Open(localTarget)
}

// Write writes a stream to the mgm
func (c *Client) Write(ctx context.Context, auth eosclient.Authorization, path string, stream io.ReadCloser) error {
	fd, err := ioutil.TempFile(c.opt.CacheDirectory, "eoswrite-")
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

	return c.WriteFile(ctx, auth, path, fd.Name())
}

// WriteFile writes an existing file to the mgm
func (c *Client) WriteFile(ctx context.Context, auth eosclient.Authorization, path, source string) error {
	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	args := []string{"--nopbar", "--silent", "-f", source, xrdPath}

	if auth.Token != "" {
		args[4] += "?authz=" + auth.Token
	} else if auth.Role.UID != "" && auth.Role.GID != "" {
		args = append(args, fmt.Sprintf("-ODeos.ruid=%s&eos.rgid=%s", auth.Role.UID, auth.Role.GID))
	}

	_, _, err := c.executeXRDCopy(ctx, args)
	return err
}

// ListDeletedEntries returns a list of the deleted entries.
func (c *Client) ListDeletedEntries(ctx context.Context, auth eosclient.Authorization) ([]*eosclient.DeletedEntry, error) {
	// TODO(labkode): add protection if slave is configured and alive to count how many files are in the trashbin before
	// triggering the recycle ls call that could break the instance because of unavailable memory.
	args := []string{"recycle", "ls", "-m"}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	if err != nil {
		return nil, err
	}
	return parseRecycleList(stdout)
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
	versionFolder := getVersionFolder(p)
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
	versionFile := path.Join(getVersionFolder(p), version)
	return c.Read(ctx, auth, versionFile)
}

// GenerateToken returns a token on behalf of the resource owner to be used by lightweight accounts
func (c *Client) GenerateToken(ctx context.Context, auth eosclient.Authorization, p string, a *acl.Entry) (string, error) {
	expiration := strconv.FormatInt(time.Now().Add(time.Duration(c.opt.TokenExpiry)*time.Second).Unix(), 10)
	args := []string{"token", "--permission", a.Permissions, "--tree", "--path", path.Clean(p) + "/", "--expires", expiration}
	stdout, _, err := c.executeEOS(ctx, args, auth)
	return stdout, err
}

func (c *Client) getVersionFolderInode(ctx context.Context, auth eosclient.Authorization, p string) (uint64, error) {
	versionFolder := getVersionFolder(p)
	md, err := c.GetFileInfoByPath(ctx, auth, versionFolder)
	if err != nil {
		if err = c.CreateDir(ctx, auth, versionFolder); err != nil {
			return 0, err
		}
		md, err = c.GetFileInfoByPath(ctx, auth, versionFolder)
		if err != nil {
			return 0, err
		}
	}
	return md.Inode, nil
}

func (c *Client) getFileInfoFromVersion(ctx context.Context, auth eosclient.Authorization, p string) (*eosclient.FileInfo, error) {
	file := getFileFromVersionFolder(p)
	md, err := c.GetFileInfoByPath(ctx, auth, file)
	if err != nil {
		return nil, err
	}
	return md, nil
}

func isVersionFolder(p string) bool {
	return strings.HasPrefix(path.Base(p), versionPrefix)
}

func getVersionFolder(p string) string {
	return path.Join(path.Dir(p), versionPrefix+path.Base(p))
}

func getFileFromVersionFolder(p string) string {
	return path.Join(path.Dir(p), strings.TrimPrefix(path.Base(p), versionPrefix))
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
// recycle=ls recycle-bin=/eos/backup/proc/recycle/ uid=gonzalhu gid=it size=381038 deletion-time=1510823151 type=file keylength.restore-path=36 restore-path=/eos/scratch/user/g/gonzalhu/app.ico restore-key=000000002544fdb3
func parseRecycleEntry(raw string) (*eosclient.DeletedEntry, error) {
	partsBySpace := strings.FieldsFunc(raw, func(c rune) bool {
		return c == ' '
	})
	restoreKeyPair, partsBySpace := partsBySpace[len(partsBySpace)-1], partsBySpace[:len(partsBySpace)-1]
	restorePathPair := strings.Join(partsBySpace[8:], " ")

	partsBySpace = partsBySpace[:8]
	partsBySpace = append(partsBySpace, restorePathPair)
	partsBySpace = append(partsBySpace, restoreKeyPair)

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

func (c *Client) parseFind(dirPath, raw string) ([]*eosclient.FileInfo, error) {
	finfos := []*eosclient.FileInfo{}
	rawLines := strings.FieldsFunc(raw, func(c rune) bool {
		return c == '\n'
	})
	for _, rl := range rawLines {
		if rl == "" {
			continue
		}
		fi, err := c.parseFileInfo(rl)
		if err != nil {
			return nil, err
		}
		// dirs in eos end with a slash, like /eos/user/g/gonzalhu/
		// we skip the current directory as eos find will return the directory we
		// ask to find
		if fi.File == path.Clean(dirPath) {
			continue
		}
		finfos = append(finfos, fi)
	}
	return finfos, nil
}

func (c Client) parseQuotaLine(line string) map[string]string {
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

		m := c.parseQuotaLine(rl)
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
				AvailableBytes:  maxBytes,
				UsedBytes:       usedBytes,
				AvailableInodes: maxInodes,
				UsedInodes:      usedInodes,
			}
			return qi, nil
		}
	}
	return &eosclient.QuotaInfo{}, nil
}

// TODO(labkode): better API to access extended attributes.
func (c *Client) parseFileInfo(raw string) (*eosclient.FileInfo, error) {

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
	// strip trailing slash
	kv["file"] = strings.TrimSuffix(name, "/")

	line = line[length+1:]
	partsBySpace := strings.FieldsFunc(line, func(c rune) bool { // we have [size=45 container=3 ...}
		return c == ' '
	})
	var previousXAttr = ""
	for _, p := range partsBySpace {
		partsByEqual := strings.Split(p, "=") // we have kv pairs like [size 14]
		if len(partsByEqual) == 2 {
			// handle xattrn and xattrv special cases
			switch {
			case partsByEqual[0] == "xattrn":
				previousXAttr = partsByEqual[1]
			case partsByEqual[0] == "xattrv":
				kv[previousXAttr] = partsByEqual[1]
				previousXAttr = ""
			default:
				kv[partsByEqual[0]] = partsByEqual[1]

			}
		}
	}
	fi, err := c.mapToFileInfo(kv)
	if err != nil {
		return nil, err
	}
	return fi, nil
}

// mapToFileInfo converts the dictionary to an usable structure.
// The kv has format:
// map[sys.forced.space:default files:0 mode:42555 ino:5 sys.forced.blocksize:4k sys.forced.layout:replica uid:0 fid:5 sys.forced.blockchecksum:crc32c sys.recycle:/eos/backup/proc/recycle/ fxid:00000005 pid:1 etag:5:0.000 keylength.file:4 file:/eos treesize:1931593933849913 container:3 gid:0 mtime:1498571294.108614409 ctime:1460121992.294326762 pxid:00000001 sys.forced.checksum:adler sys.forced.nstripes:2]
func (c *Client) mapToFileInfo(kv map[string]string) (*eosclient.FileInfo, error) {
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
		if mtimesec, err = strconv.ParseUint(mtimeSplit[0], 10, 64); err != nil {
			return nil, err
		}
		if mtimenanos, err = strconv.ParseUint(mtimeSplit[1], 10, 32); err != nil {
			return nil, err
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

	sysACL, err := acl.Parse(kv["sys.acl"], acl.ShortTextForm)
	if err != nil {
		return nil, err
	}
	lwACLStr, ok := kv[lwShareAttrKey]
	if ok {
		lwAcls, err := acl.Parse(lwACLStr, acl.ShortTextForm)
		if err != nil {
			return nil, err
		}
		for _, e := range lwAcls.Entries {
			err = sysACL.SetEntry(e.Type, e.Qualifier, e.Permissions)
			if err != nil {
				return nil, err
			}
		}
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
		IsDir:      isDir,
		Instance:   c.opt.URL,
		SysACL:     sysACL,
		TreeCount:  treeCount,
		Attrs:      kv,
		XS:         xs,
	}

	return fi, nil
}
