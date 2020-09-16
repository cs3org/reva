// Copyright 2018-2020 CERN
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

package eosclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/acl"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

const (
	versionPrefix = ".sys.v#."

	versionAquamarine = eosVersion("aquamarine")
	versionCitrine    = eosVersion("citrine")
)

// AttrType is the type of extended attribute,
// either system (sys) or user (user).
type AttrType uint32

type eosVersion string

const (
	// SystemAttr is the system extended attribute.
	SystemAttr AttrType = iota
	// UserAttr is the user extended attribute.
	UserAttr
)

func (at AttrType) String() string {
	switch at {
	case SystemAttr:
		return "sys"
	case UserAttr:
		return "user"
	default:
		return "invalid"
	}
}

// Attribute represents an EOS extended attribute.
type Attribute struct {
	Type     AttrType
	Key, Val string
}

func (a *Attribute) serialize() string {
	return fmt.Sprintf("%s.%s=%s", a.Type, a.Key, a.Val)
}

func (a *Attribute) isValid() bool {
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
	// Default is /usr/bin/xrdcopy.
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
}

func (opt *Options) init() {
	if opt.ForceSingleUserMode && opt.SingleUsername != "" {
		opt.SingleUsername = "apache"
	}

	if opt.EosBinary == "" {
		opt.EosBinary = "/usr/bin/eos"
	}

	if opt.XrdcopyBinary == "" {
		opt.XrdcopyBinary = "/usr/bin/xrdcopy"
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
func New(opt *Options) *Client {
	opt.init()
	c := new(Client)
	c.opt = opt
	return c
}

// exec executes the command and returns the stdout, stderr and return code
func (c *Client) execute(ctx context.Context, cmd *exec.Cmd) (string, string, error) {
	log := appctx.GetLogger(ctx)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
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
			case 2:
				err = errtypes.NotFound(errBuf.String())
			}
		}
	}

	args := fmt.Sprintf("%s", cmd.Args)
	env := fmt.Sprintf("%s", cmd.Env)
	log.Info().Str("args", args).Str("env", env).Int("exit", exitStatus).Msg("eos cmd")

	if err != nil && exitStatus != 2 { // don't wrap the errtypes.NotFoundError
		err = errors.Wrap(err, "eosclient: error while executing command")
	}

	return outBuf.String(), errBuf.String(), err
}

// exec executes only EOS commands the command and returns the stdout, stderr and return code.
// execute() executes arbitrary commands.
func (c *Client) executeEOS(ctx context.Context, cmd *exec.Cmd) (string, string, error) {
	log := appctx.GetLogger(ctx)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	cmd.Env = []string{
		"EOS_MGM_URL=" + c.opt.URL,
	}
	if c.opt.UseKeytab {
		cmd.Env = append(cmd.Env, "XrdSecPROTOCOL="+c.opt.SecProtocol)
		cmd.Env = append(cmd.Env, "XrdSecSSSKT="+c.opt.Keytab)
	}

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
			case 2:
				err = errtypes.NotFound(errBuf.String())
			case 1, 22:
				// eos reports back error code 22 when the user is not allowed to enter the instance
				err = errtypes.PermissionDenied(errBuf.String())
			}
		}
	}

	args := fmt.Sprintf("%s", cmd.Args)
	env := fmt.Sprintf("%s", cmd.Env)
	log.Info().Str("args", args).Str("env", env).Int("exit", exitStatus).Str("err", errBuf.String()).Msg("eos cmd")

	if err != nil && exitStatus != 2 { // don't wrap the errtypes.NotFoundError
		err = errors.Wrap(err, "eosclient: error while executing command")
	}

	return outBuf.String(), errBuf.String(), err
}

func (c *Client) getVersion(ctx context.Context, rootUID, rootGID string) (eosVersion, error) {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", rootUID, rootGID, "version")
	stdout, _, err := c.executeEOS(ctx, cmd)
	if err != nil {
		return "", err
	}
	return c.parseVersion(ctx, stdout), nil
}

func (c *Client) parseVersion(ctx context.Context, raw string) eosVersion {
	var serverVersion string
	rawLines := strings.Split(raw, "\n")
	for _, rl := range rawLines {
		if rl == "" {
			continue
		}
		if strings.HasPrefix(rl, "EOS_SERVER_VERSION") {
			serverVersion = strings.Split(strings.Split(rl, " ")[0], "=")[1]
			break
		}
	}

	if strings.HasPrefix(serverVersion, "4.") {
		return versionCitrine
	}
	return versionAquamarine
}

// AddACL adds an new acl to EOS with the given aclType.
func (c *Client) AddACL(ctx context.Context, uid, gid, rootUID, rootGID, path string, a *acl.Entry) error {
	version, err := c.getVersion(ctx, rootUID, rootGID)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if version == versionCitrine {
		sysACL := a.CitrineSerialize()
		cmd = exec.CommandContext(ctx, c.opt.EosBinary, "-r", rootUID, rootGID, "acl", "--sys", "--recursive", sysACL, path)
	} else {
		acls, err := c.getACLForPath(ctx, uid, gid, path)
		if err != nil {
			return err
		}

		err = acls.SetEntry(a.Type, a.Qualifier, a.Permissions)
		if err != nil {
			return err
		}
		sysACL := acls.Serialize()
		cmd = exec.CommandContext(ctx, c.opt.EosBinary, "-r", rootUID, rootGID, "attr", "-r", "set", fmt.Sprintf("sys.acl=%s", sysACL), path)
	}

	_, _, err = c.executeEOS(ctx, cmd)
	return err

}

// RemoveACL removes the acl from EOS.
func (c *Client) RemoveACL(ctx context.Context, uid, gid, rootUID, rootGID, path string, a *acl.Entry) error {
	version, err := c.getVersion(ctx, rootUID, rootGID)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if version == versionCitrine {
		sysACL := a.CitrineSerialize()
		cmd = exec.CommandContext(ctx, c.opt.EosBinary, "-r", rootUID, rootGID, "acl", "--sys", "--recursive", sysACL, path)
	} else {
		acls, err := c.getACLForPath(ctx, uid, gid, path)
		if err != nil {
			return err
		}

		acls.DeleteEntry(a.Type, a.Qualifier)
		sysACL := acls.Serialize()
		cmd = exec.CommandContext(ctx, c.opt.EosBinary, "-r", rootUID, rootGID, "attr", "-r", "set", fmt.Sprintf("sys.acl=%s", sysACL), path)
	}

	_, _, err = c.executeEOS(ctx, cmd)
	return err

}

// UpdateACL updates the EOS acl.
func (c *Client) UpdateACL(ctx context.Context, uid, gid, rootUID, rootGID, path string, a *acl.Entry) error {
	return c.AddACL(ctx, uid, gid, rootUID, rootGID, path, a)
}

// GetACL for a file
func (c *Client) GetACL(ctx context.Context, uid, gid, path, aclType, target string) (*acl.Entry, error) {
	acls, err := c.ListACLs(ctx, uid, gid, path)
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
func (c *Client) ListACLs(ctx context.Context, uid, gid, path string) ([]*acl.Entry, error) {

	parsedACLs, err := c.getACLForPath(ctx, uid, gid, path)
	if err != nil {
		return nil, err
	}

	// EOS Citrine ACLs are stored with uid. The UID will be resolved to the
	// user opaque ID at the eosfs level.
	return parsedACLs.Entries, nil
}

func (c *Client) getACLForPath(ctx context.Context, uid, gid, path string) (*acl.ACLs, error) {
	finfo, err := c.GetFileInfoByPath(ctx, uid, gid, path)
	if err != nil {
		return nil, err
	}

	return acl.Parse(finfo.SysACL, acl.ShortTextForm)
}

// GetFileInfoByInode returns the FileInfo by the given inode
func (c *Client) GetFileInfoByInode(ctx context.Context, uid, gid string, inode uint64) (*FileInfo, error) {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "file", "info", fmt.Sprintf("inode:%d", inode), "-m")
	stdout, _, err := c.executeEOS(ctx, cmd)
	if err != nil {
		return nil, err
	}
	info, err := c.parseFileInfo(stdout)
	if err != nil {
		return nil, err
	}

	if c.opt.VersionInvariant {
		info, err = c.getFileInfoFromVersion(ctx, uid, gid, info.File)
		if err != nil {
			return nil, err
		}
		info.Inode = inode
	}

	return info, nil
}

// GetFileInfoByFXID returns the FileInfo by the given file id in hexadecimal
func (c *Client) GetFileInfoByFXID(ctx context.Context, uid, gid string, fxid string) (*FileInfo, error) {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "file", "info", fmt.Sprintf("fxid:%s", fxid), "-m")
	stdout, _, err := c.executeEOS(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return c.parseFileInfo(stdout)
}

// SetAttr sets an extended attributes on a path.
func (c *Client) SetAttr(ctx context.Context, uid, gid string, attr *Attribute, recursive bool, path string) error {
	if !attr.isValid() {
		return errors.New("eos: attr is invalid: " + attr.serialize())
	}
	var cmd *exec.Cmd
	if recursive {
		cmd = exec.CommandContext(ctx, "/usr/bin/eos", "-r", uid, gid, "attr", "-r", "set", attr.serialize(), path)
	} else {
		cmd = exec.CommandContext(ctx, "/usr/bin/eos", "-r", uid, gid, "attr", "set", attr.serialize(), path)
	}

	_, _, err := c.executeEOS(ctx, cmd)
	if err != nil {
		return err
	}
	return nil
}

// UnsetAttr unsets an extended attribute on a path.
func (c *Client) UnsetAttr(ctx context.Context, uid, gid string, attr *Attribute, path string) error {
	if !attr.isValid() {
		return errors.New("eos: attr is invalid: " + attr.serialize())
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", uid, gid, "attr", "-r", "rm", fmt.Sprintf("%s.%s", attr.Type, attr.Key), path)
	_, _, err := c.executeEOS(ctx, cmd)
	if err != nil {
		return err
	}
	return nil
}

// GetFileInfoByPath returns the FilInfo at the given path
func (c *Client) GetFileInfoByPath(ctx context.Context, uid, gid, path string) (*FileInfo, error) {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "file", "info", path, "-m")
	stdout, _, err := c.executeEOS(ctx, cmd)
	if err != nil {
		return nil, err
	}
	info, err := c.parseFileInfo(stdout)
	if err != nil {
		return nil, err
	}

	if c.opt.VersionInvariant {
		inode, err := c.getVersionFolderInode(ctx, uid, gid, path)
		if err != nil {
			return nil, err
		}
		info.Inode = inode
	}

	return info, nil
}

// GetQuota gets the quota of a user on the quota node defined by path
func (c *Client) GetQuota(ctx context.Context, username, rootUID, rootGID, path string) (*QuotaInfo, error) {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", rootUID, rootGID, "quota", "ls", "-u", username, "-m")
	stdout, _, err := c.executeEOS(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return c.parseQuota(path, stdout)
}

// Touch creates a 0-size,0-replica file in the EOS namespace.
func (c *Client) Touch(ctx context.Context, uid, gid, path string) error {
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", uid, gid, "file", "touch", path)
	_, _, err := c.executeEOS(ctx, cmd)
	return err
}

// Chown given path
func (c *Client) Chown(ctx context.Context, uid, gid, chownUID, chownGID, path string) error {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "chown", chownUID+":"+chownGID, path)
	_, _, err := c.executeEOS(ctx, cmd)
	return err
}

// Chmod given path
func (c *Client) Chmod(ctx context.Context, uid, gid, mode, path string) error {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "chmod", mode, path)
	_, _, err := c.executeEOS(ctx, cmd)
	return err
}

// CreateDir creates a directory at the given path
func (c *Client) CreateDir(ctx context.Context, uid, gid, path string) error {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "mkdir", "-p", path)
	_, _, err := c.executeEOS(ctx, cmd)
	return err
}

// Remove removes the resource at the given path
func (c *Client) Remove(ctx context.Context, uid, gid, path string) error {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "rm", "-r", path)
	_, _, err := c.executeEOS(ctx, cmd)
	return err
}

// Rename renames the resource referenced by oldPath to newPath
func (c *Client) Rename(ctx context.Context, uid, gid, oldPath, newPath string) error {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "file", "rename", oldPath, newPath)
	_, _, err := c.executeEOS(ctx, cmd)
	return err
}

// List the contents of the directory given by path
func (c *Client) List(ctx context.Context, uid, gid, path string) ([]*FileInfo, error) {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "find", "--fileinfo", "--maxdepth", "1", path)
	stdout, _, err := c.executeEOS(ctx, cmd)
	if err != nil {
		return nil, errors.Wrapf(err, "eosclient: error listing fn=%s", path)
	}
	return c.parseFind(path, stdout)
}

// Read reads a file from the mgm
func (c *Client) Read(ctx context.Context, uid, gid, path string) (io.ReadCloser, error) {
	uuid := uuid.Must(uuid.NewV4())
	rand := "eosread-" + uuid.String()
	localTarget := fmt.Sprintf("%s/%s", c.opt.CacheDirectory, rand)
	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	cmd := exec.CommandContext(ctx, c.opt.XrdcopyBinary, "--nopbar", "--silent", "-f", xrdPath, localTarget, fmt.Sprintf("-OSeos.ruid=%s&eos.rgid=%s", uid, gid))
	_, _, err := c.execute(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return os.Open(localTarget)
}

// Write writes a stream to the mgm
func (c *Client) Write(ctx context.Context, uid, gid, path string, stream io.ReadCloser) error {
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

	return c.WriteFile(ctx, uid, gid, path, fd.Name())
}

// WriteFile writes an existing file to the mgm
func (c *Client) WriteFile(ctx context.Context, uid, gid, path, source string) error {
	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	cmd := exec.CommandContext(ctx, c.opt.XrdcopyBinary, "--nopbar", "--silent", "-f", source, xrdPath, fmt.Sprintf("-ODeos.ruid=%s&eos.rgid=%s", uid, gid))
	_, _, err := c.execute(ctx, cmd)
	return err
}

// ListDeletedEntries returns a list of the deleted entries.
func (c *Client) ListDeletedEntries(ctx context.Context, uid, gid string) ([]*DeletedEntry, error) {
	// TODO(labkode): add protection if slave is configured and alive to count how many files are in the trashbin before
	// triggering the recycle ls call that could break the instance because of unavailable memory.
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "recycle", "ls", "-m")
	stdout, _, err := c.executeEOS(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return parseRecycleList(stdout)
}

// RestoreDeletedEntry restores a deleted entry.
func (c *Client) RestoreDeletedEntry(ctx context.Context, uid, gid, key string) error {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "recycle", "restore", key)
	_, _, err := c.executeEOS(ctx, cmd)
	return err
}

// PurgeDeletedEntries purges all entries from the recycle bin.
func (c *Client) PurgeDeletedEntries(ctx context.Context, uid, gid string) error {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "recycle", "purge")
	_, _, err := c.executeEOS(ctx, cmd)
	return err
}

// ListVersions list all the versions for a given file.
func (c *Client) ListVersions(ctx context.Context, uid, gid, p string) ([]*FileInfo, error) {
	versionFolder := getVersionFolder(p)
	finfos, err := c.List(ctx, uid, gid, versionFolder)
	if err != nil {
		// we send back an empty list
		return []*FileInfo{}, nil
	}
	return finfos, nil
}

// RollbackToVersion rollbacks a file to a previous version.
func (c *Client) RollbackToVersion(ctx context.Context, uid, gid, path, version string) error {
	cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "file", "versions", path, version)
	_, _, err := c.executeEOS(ctx, cmd)
	return err
}

// ReadVersion reads the version for the given file.
func (c *Client) ReadVersion(ctx context.Context, uid, gid, p, version string) (io.ReadCloser, error) {
	versionFile := path.Join(getVersionFolder(p), version)
	return c.Read(ctx, uid, gid, versionFile)
}

func (c *Client) getVersionFolderInode(ctx context.Context, uid, gid, p string) (uint64, error) {
	versionFolder := getVersionFolder(p)
	md, err := c.GetFileInfoByPath(ctx, uid, gid, versionFolder)
	if err != nil {
		if err = c.CreateDir(ctx, uid, gid, versionFolder); err != nil {
			return 0, err
		}
		md, err = c.GetFileInfoByPath(ctx, uid, gid, versionFolder)
		if err != nil {
			return 0, err
		}
	}
	return md.Inode, nil
}

func (c *Client) getFileInfoFromVersion(ctx context.Context, uid, gid, p string) (*FileInfo, error) {
	file := getFileFromVersionFolder(p)
	md, err := c.GetFileInfoByPath(ctx, uid, gid, file)
	if err != nil {
		return nil, err
	}
	return md, nil
}

func getVersionFolder(p string) string {
	return path.Join(path.Dir(p), versionPrefix+path.Base(p))
}

func getFileFromVersionFolder(p string) string {
	return path.Join(path.Dir(p), strings.TrimPrefix(path.Base(p), versionPrefix))
}

func parseRecycleList(raw string) ([]*DeletedEntry, error) {
	entries := []*DeletedEntry{}
	rawLines := strings.Split(raw, "\n")
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
func parseRecycleEntry(raw string) (*DeletedEntry, error) {
	partsBySpace := strings.Split(raw, " ")
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
	entry := &DeletedEntry{
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

func (c *Client) parseFind(dirPath, raw string) ([]*FileInfo, error) {
	finfos := []*FileInfo{}
	rawLines := strings.Split(raw, "\n")
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
	partsBySpace := strings.Split(line, " ")
	m := getMap(partsBySpace)
	return m
}
func (c *Client) parseQuota(path, raw string) (*QuotaInfo, error) {
	rawLines := strings.Split(raw, "\n")
	for _, rl := range rawLines {
		if rl == "" {
			continue
		}

		m := c.parseQuotaLine(rl)
		// map[maxbytes:2000000000000 maxlogicalbytes:1000000000000 percentageusedbytes:0.49 quota:node uid:gonzalhu space:/eos/scratch/user/ usedbytes:9829986500 usedlogicalbytes:4914993250 statusfiles:ok usedfiles:334 maxfiles:1000000 statusbytes:ok]

		space := m["space"]
		if strings.HasPrefix(path, space) {
			maxBytesString := m["maxlogicalbytes"]
			usedBytesString := m["usedlogicalbytes"]
			maxBytes, _ := strconv.ParseInt(maxBytesString, 10, 64)
			usedBytes, _ := strconv.ParseInt(usedBytesString, 10, 64)

			maxInodesString := m["maxfiles"]
			usedInodesString := m["usedfiles"]
			maxInodes, _ := strconv.ParseInt(maxInodesString, 10, 64)
			usedInodes, _ := strconv.ParseInt(usedInodesString, 10, 64)

			qi := &QuotaInfo{
				AvailableBytes:  int(maxBytes),
				UsedBytes:       int(usedBytes),
				AvailableInodes: int(maxInodes),
				UsedInodes:      int(usedInodes),
			}
			return qi, nil
		}
	}
	return &QuotaInfo{}, nil
}

// TODO(labkode): better API to access extended attributes.
func (c *Client) parseFileInfo(raw string) (*FileInfo, error) {

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
	partsBySpace := strings.Split(line, " ") // we have [size=45 container=3 ...}
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
func (c *Client) mapToFileInfo(kv map[string]string) (*FileInfo, error) {
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

	// mtime is split by a dot, we only take the first part, do we need subsec precision?
	mtime := strings.Split(kv["mtime"], ".")
	mtimesec, err := strconv.ParseUint(mtime[0], 10, 64)
	if err != nil {
		return nil, err
	}
	mtimenanos, err := strconv.ParseUint(mtime[1], 10, 32)
	if err != nil {
		return nil, err
	}

	isDir := false
	if _, ok := kv["files"]; ok {
		isDir = true
	}

	fi := &FileInfo{
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
		SysACL:     kv["sys.acl"],
		TreeCount:  treeCount,
		Attrs:      kv,
	}

	return fi, nil
}

// FileInfo represents the metadata information returned by querying the EOS namespace.
type FileInfo struct {
	IsDir      bool
	MTimeNanos uint32
	Inode      uint64            `json:"inode"`
	FID        uint64            `json:"fid"`
	UID        uint64            `json:"uid"`
	GID        uint64            `json:"gid"`
	TreeSize   uint64            `json:"tree_size"`
	MTimeSec   uint64            `json:"mtime_sec"`
	Size       uint64            `json:"size"`
	TreeCount  uint64            `json:"tree_count"`
	File       string            `json:"eos_file"`
	ETag       string            `json:"etag"`
	Instance   string            `json:"instance"`
	SysACL     string            `json:"sys_acl"`
	Attrs      map[string]string `json:"attrs"`
}

// DeletedEntry represents an entry from the trashbin.
type DeletedEntry struct {
	RestorePath   string
	RestoreKey    string
	Size          uint64
	DeletionMTime uint64
	IsDir         bool
}

// QuotaInfo reports the available bytes and inodes for a particular user.
type QuotaInfo struct {
	AvailableBytes, UsedBytes   int
	AvailableInodes, UsedInodes int
}
