package eosclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	gouser "os/user"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/cernbox/reva/pkg/log"

	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
)

const (
	rootUser      = "root"
	rootGroup     = "root"
	versionPrefix = ".sys.v#."
)

/*
type ACLMode string

// ACLType represents the type of of the acl (user, e-group, unix-group, ...)
type ACLType string

const (
	// ACLModeInvalid specifies an invalid acl mode.
	ACLModeInvalid = ACLMode("invalid")
	// ACLModeRead specifies that only read and list operations will be allowed on the directory.
	ACLModeRead = ACLMode("rx")
	// ACLModeReadWrite specifies that the directory will be writable.
	ACLModeReadWrite = ACLMode("rwx!d")

	ACLTypeUnknown = ACLType(iota)
	// ACLTypeUser specifies that the acl will be set for an individual user.
	ACLTypeUser
	// ACLTypeGroup specifies that the acl will be set for a CERN e-group.
	ACLTypeGroup
	// ACLTypeUnixGroup specifies that the acl will be set for a unix group.
	ACLTypeUnixGroup

	rootUser      = "root"
	rootGroup     = "root"
	versionPrefix = ".sys.v#."
)
*/

var (
	errInvalidACL = errors.New("invalid acl")
)

// ACL represents an EOS ACL.
type ACL struct {
	Target string
	Mode   string
	Type   string
}

// Options to configure the Client.
type Options struct {
	// Location of the eos binary.
	// Default is /usr/bin/eos.
	EosBinary string

	// Location of the xrdcopy binary.
	// Default is /usr/bin/xrdcopy.
	XrdcopyBinary string

	// URL of the EOS MGM.
	// Default is root://eos-test.org
	URL string

	// Location on the local fs where to store reads.
	// Defaults to os.TempDir()
	CacheDirectory string

	// Writter to write logs to
	LogOutput io.Writer

	// Key to get the trace Id from.
	TraceKey interface{}
}

func (opt *Options) init() {
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

	if opt.LogOutput == nil {
		opt.LogOutput = ioutil.Discard
	}

	if opt.TraceKey == nil {
		opt.TraceKey = "traceid"
	}
}

// Client performs actions against a EOS management node (MGM).
// It requires the eos-client and xrootd-client packages installed to work.
type Client struct {
	opt    *Options
	logger *log.Logger
}

// New creates a new client with the given options.
func New(opt *Options) *Client {
	opt.init()
	c := new(Client)
	c.opt = opt
	c.logger = log.New("eosclient")
	return c
}

func getUnixUser(username string) (*gouser.User, error) {
	return gouser.Lookup(username)
}

// exec executes the command and returns the stdout, stderr and return code
func (c *Client) execute(ctx context.Context, cmd *exec.Cmd) (string, string, error) {
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	cmd.Env = []string{
		"EOS_MGM_URL=" + c.opt.URL,
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
				err = notFoundError(errBuf.String())
			case 22:
				// eos reports back error code 22 when the user is not allowed to enter the instance
				err = notFoundError(errBuf.String())
			}
		}
	}

	msg := fmt.Sprintf("cmd=%v env=%v exit=%d", cmd.Args, cmd.Env, exitStatus)
	c.logger.Println(ctx, msg)

	if err != nil {
		err = errors.Wrap(err, "eosclient: error while executing command")
	}

	return outBuf.String(), errBuf.String(), err
}

// AddACL adds an new acl to EOS with the given aclType.
func (c *Client) AddACL(ctx context.Context, username, path string, a *ACL) error {
	aclManager, err := c.getACLForPath(ctx, username, path)
	if err != nil {
		return err
	}

	aclManager.deleteEntry(ctx, a.Type, a.Target)
	newEntry, err := newACLEntry(ctx, strings.Join([]string{a.Type, a.Target, a.Mode}, ":"))
	if err != nil {
		return err
	}
	aclManager.aclEntries = append(aclManager.aclEntries, newEntry)
	sysACL := aclManager.serialize()

	// setting of the sys.acl is only possible from root user
	unixUser, err := getUnixUser(rootUser)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "attr", "-r", "set", fmt.Sprintf("sys.acl=%s", sysACL), path)
	_, _, err = c.execute(ctx, cmd)
	return err

}

// deleteEntry will be called with username but acl is stored with uid, we need to convert back uid
// to username.
func (m *aclManager) deleteEntry(ctx context.Context, aclType, target string) {
	for i, e := range m.aclEntries {
		username, err := getUsername(e.recipient)
		if err != nil {
			continue
		}
		if username == target && e.aclType == aclType {
			m.aclEntries = append(m.aclEntries[:i], m.aclEntries[i+1:]...)
			return
		}
	}
}

// RemoveACL removes the acl from EOS.
func (c *Client) RemoveACL(ctx context.Context, username, path string, aclType string, recipient string) error {
	aclManager, err := c.getACLForPath(ctx, username, path)
	if err != nil {
		return err
	}

	aclManager.deleteEntry(ctx, aclType, recipient)
	sysACL := aclManager.serialize()

	// setting of the sys.acl is only possible from root user
	unixUser, err := getUnixUser(rootUser)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "attr", "-r", "set", fmt.Sprintf("sys.acl=%s", sysACL), path)
	_, _, err = c.execute(ctx, cmd)
	return err

}

// UpdateACL updates the EOS acl.
func (c *Client) UpdateACL(ctx context.Context, username, path string, a *ACL) error {
	return c.AddACL(ctx, username, path, a)
}

func (c *Client) GetACL(ctx context.Context, username, path, aclType, target string) (*ACL, error) {
	acls, err := c.ListACLs(ctx, username, path)
	if err != nil {
		return nil, err
	}
	for _, a := range acls {
		if a.Type == aclType && a.Target == target {
			return a, nil
		}
	}
	return nil, notFoundError(fmt.Sprintf("%s:%s", aclType, target))

}

func getUsername(uid string) (string, error) {
	user, err := gouser.LookupId(uid)
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

// ListACLS returns the list of ACLs present under the given path.
// EOS returns uids/gid for Citrine version and usernames for older versions.
// For Citire we need to convert back the uid back to username.
func (c *Client) ListACLs(ctx context.Context, username, path string) ([]*ACL, error) {
	finfo, err := c.GetFileInfoByPath(ctx, username, path)
	if err != nil {
		return nil, err
	}

	aclManager := c.newACLManager(ctx, finfo.SysACL)
	acls := []*ACL{}
	for _, a := range aclManager.getEntries() {
		username, err := getUsername(a.recipient)
		if err != nil {
			c.logger.Error(ctx, err)
			continue
		}
		acl := &ACL{
			Target: username,
			Mode:   a.mode,
			Type:   a.aclType,
		}
		acls = append(acls, acl)
	}
	return acls, nil
}

func (c *Client) getACLForPath(ctx context.Context, username, path string) (*aclManager, error) {
	finfo, err := c.GetFileInfoByPath(ctx, username, path)
	if err != nil {
		return nil, err
	}

	aclManager := c.newACLManager(ctx, finfo.SysACL)
	return aclManager, nil
}

// GetFileInfoByInode returns the FileInfo by the given inode
func (c *Client) GetFileInfoByInode(ctx context.Context, username string, inode uint64) (*FileInfo, error) {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "file", "info", fmt.Sprintf("inode:%d", inode), "-m")
	stdout, _, err := c.execute(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return c.parseFileInfo(stdout)
}

// GetFileInfoByPath returns the FilInfo at the given path
func (c *Client) GetFileInfoByPath(ctx context.Context, username, path string) (*FileInfo, error) {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "file", "info", path, "-m")
	stdout, _, err := c.execute(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return c.parseFileInfo(stdout)
}

// GetQuota gets the quota of a user on the quota node defined by path
func (c *Client) GetQuota(ctx context.Context, username, path string) (int, int, error) {
	// setting of the sys.acl is only possible from root user
	unixUser, err := getUnixUser(rootUser)
	if err != nil {
		return 0, 0, err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "quota", "ls", "-u", username, "-m")
	stdout, _, err := c.execute(ctx, cmd)
	if err != nil {
		return 0, 0, err
	}
	return c.parseQuota(path, stdout)
}

// CreateDir creates a directory at the given path
func (c *Client) CreateDir(ctx context.Context, username, path string) error {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "mkdir", "-p", path)
	_, _, err = c.execute(ctx, cmd)
	return err
}

// Remove removes the resource at the given path
func (c *Client) Remove(ctx context.Context, username, path string) error {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "rm", "-r", path)
	_, _, err = c.execute(ctx, cmd)
	return err
}

// Rename renames the resource referenced by oldPath to newPath
func (c *Client) Rename(ctx context.Context, username, oldPath, newPath string) error {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "file", "rename", oldPath, newPath)
	_, _, err = c.execute(ctx, cmd)
	return err
}

// List the contents of the directory given by path
func (c *Client) List(ctx context.Context, username, path string) ([]*FileInfo, error) {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "find", "--fileinfo", "--maxdepth", "1", path)
	stdout, _, err := c.execute(ctx, cmd)
	if err != nil {
		return nil, errors.Wrapf(err, "eosclient: error listing fn=%s", path)
	}
	return c.parseFind(path, stdout)
}

// Read reads a file from the mgm
func (c *Client) Read(ctx context.Context, username, path string) (io.ReadCloser, error) {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return nil, err
	}
	uuid := uuid.Must(uuid.NewV4())
	rand := "eosread-" + uuid.String()
	localTarget := fmt.Sprintf("%s/%s", c.opt.CacheDirectory, rand)
	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	cmd := exec.CommandContext(ctx, "/usr/bin/xrdcopy", "--nopbar", "--silent", "-f", xrdPath, localTarget, fmt.Sprintf("-OSeos.ruid=%s&eos.rgid=%s", unixUser.Uid, unixUser.Gid))
	_, _, err = c.execute(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return os.Open(localTarget)
}

// Write writes a file to the mgm
func (c *Client) Write(ctx context.Context, username, path string, stream io.ReadCloser) error {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return err
	}
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
	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	cmd := exec.CommandContext(ctx, "/usr/bin/xrdcopy", "--nopbar", "--silent", "-f", fd.Name(), xrdPath, fmt.Sprintf("-ODeos.ruid=%s&eos.rgid=%s", unixUser.Uid, unixUser.Gid))
	_, _, err = c.execute(ctx, cmd)
	return err
}

// ListDeletedEntries returns a list of the deleted entries.
func (c *Client) ListDeletedEntries(ctx context.Context, username string) ([]*DeletedEntry, error) {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return nil, err
	}
	// TODO(labkode): add protection if slave is configured and alive to count how many files are in the trashbin before
	// triggering the recycle ls call that could break the instance because of unavailable memory.
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "recycle", "ls", "-m")
	stdout, _, err := c.execute(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return parseRecycleList(stdout)
}

// RestoreDeletedEntry restores a deleted entry.
func (c *Client) RestoreDeletedEntry(ctx context.Context, username, key string) error {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "recycle", "restore", key)
	_, _, err = c.execute(ctx, cmd)
	return err
}

// PurgeDeletedEntries purges all entries from the recycle bin.
func (c *Client) PurgeDeletedEntries(ctx context.Context, username string) error {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "recycle", "purge")
	_, _, err = c.execute(ctx, cmd)
	return err
}

func getVersionFolder(p string) string {
	basename := path.Base(p)
	versionFolder := path.Join(path.Dir(p), versionPrefix+basename)
	return versionFolder
}

// ListVersions list all the versions for a given file.
func (c *Client) ListVersions(ctx context.Context, username, p string) ([]*FileInfo, error) {
	basename := path.Base(p)
	versionFolder := path.Join(path.Dir(p), versionPrefix+basename)
	finfos, err := c.List(ctx, username, versionFolder)
	if err != nil {
		// we send back an empty list
		return []*FileInfo{}, nil
	}
	return finfos, nil
}

// RollbackToVersion rollbacks a file to a previous version.
func (c *Client) RollbackToVersion(ctx context.Context, username, path, version string) error {
	unixUser, err := getUnixUser(username)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/eos", "-r", unixUser.Uid, unixUser.Gid, "file", "versions", path, version)
	_, _, err = c.execute(ctx, cmd)
	return err
}

// ReadVersion reads the version for the given file.
func (c *Client) ReadVersion(ctx context.Context, username, p, version string) (io.ReadCloser, error) {
	basename := path.Base(p)
	versionFile := path.Join(path.Dir(p), versionPrefix+basename, version)
	return c.Read(ctx, username, versionFile)
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
// recycle=ls  recycle-bin=/eos/backup/proc/recycle/ uid=gonzalhu gid=it size=0 deletion-time=1510823151 type=recursive-dir keylength.restore-path=45 restore-path=/eos/scratch/user/g/gonzalhu/.sys.v#.app.ico/ restore-key=0000000000a35100
// recycle=ls  recycle-bin=/eos/backup/proc/recycle/ uid=gonzalhu gid=it size=381038 deletion-time=1510823151 type=file keylength.restore-path=36 restore-path=/eos/scratch/user/g/gonzalhu/app.ico restore-key=000000002544fdb3
func parseRecycleEntry(raw string) (*DeletedEntry, error) {
	partsBySpace := strings.Split(raw, " ")
	restoreKeyPair, partsBySpace := partsBySpace[len(partsBySpace)-1], partsBySpace[:len(partsBySpace)-1]
	restorePathPair := strings.Join(partsBySpace[9:], " ")

	partsBySpace = partsBySpace[:9]
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
func (c *Client) parseQuota(path, raw string) (int, int, error) {
	rawLines := strings.Split(raw, "\n")
	for _, rl := range rawLines {
		if rl == "" {
			continue
		}

		m := c.parseQuotaLine(rl)
		// map[maxbytes:2000000000000 maxlogicalbytes:1000000000000 percentageusedbytes:0.49 quota:node uid:gonzalhu space:/eos/scratch/user/ usedbytes:9829986500 usedlogicalbytes:4914993250 statusfiles:ok usedfiles:334 maxfiles:1000000 statusbytes:ok]

		space := m["space"]
		if strings.HasPrefix(path, space) {
			maxBytesString, _ := m["maxlogicalbytes"]
			usedBytesString, _ := m["usedlogicalbytes"]
			maxBytes, _ := strconv.ParseInt(maxBytesString, 10, 64)
			usedBytes, _ := strconv.ParseInt(usedBytesString, 10, 64)
			return int(maxBytes), int(usedBytes), nil
		}
	}
	return 0, 0, nil
}

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
			if partsByEqual[0] == "xattrn" {
				previousXAttr = partsByEqual[1]
			} else if partsByEqual[0] == "xattrv" {
				kv[previousXAttr] = partsByEqual[1]
				previousXAttr = ""
			} else {
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
	mtime, err := strconv.ParseUint(strings.Split(kv["mtime"], ".")[0], 10, 64)
	if err != nil {
		return nil, err
	}

	isDir := false
	if _, ok := kv["files"]; ok {
		isDir = true
	}

	fi := &FileInfo{
		File:      kv["file"],
		Inode:     inode,
		FID:       fid,
		ETag:      kv["etag"],
		Size:      size,
		TreeSize:  treeSize,
		MTime:     mtime,
		IsDir:     isDir,
		Instance:  c.opt.URL,
		SysACL:    kv["sys.acl"],
		TreeCount: treeCount,
	}
	return fi, nil
}

// FileInfo represents the metadata information returned by querying the EOS namespace.
type FileInfo struct {
	File      string `json:"eos_file"`
	Inode     uint64 `json:"inode"`
	FID       uint64 `json:"fid"`
	ETag      string
	TreeSize  uint64
	MTime     uint64
	Size      uint64
	IsDir     bool
	Instance  string
	SysACL    string
	TreeCount uint64
}

// DeletedEntry represents an entry from the trashbin.
type DeletedEntry struct {
	RestorePath   string
	RestoreKey    string
	Size          uint64
	DeletionMTime uint64
	IsDir         bool
}

type aclManager struct {
	aclEntries []*aclEntry
}

func (c *Client) newACLManager(ctx context.Context, sysACL string) *aclManager {
	tokens := strings.Split(sysACL, ",")
	aclEntries := []*aclEntry{}
	for _, t := range tokens {
		aclEntry, err := newACLEntry(ctx, t)
		if err == nil {
			aclEntries = append(aclEntries, aclEntry)
		}
	}

	return &aclManager{aclEntries: aclEntries}
}

func (m *aclManager) getEntries() []*aclEntry {
	return m.aclEntries
}

/*
func (m *aclManager) getUsers() []*aclEntry {
	entries := []*aclEntry{}
	for _, e := range m.aclEntries {
		if e.aclType == ACLTypeUser {
			entries = append(entries, e)
		}
	}
	return entries
}

func (m *aclManager) getUsersWithReadPermission() []*aclEntry {
	entries := []*aclEntry{}
	for _, e := range m.aclEntries {
		if e.aclType == ACLTypeUser && e.hasReadPermissions() {
			entries = append(entries, e)
		}
	}
	return entries
}

func (m *aclManager) getUsersWithWritePermission() []*aclEntry {
	entries := []*aclEntry{}
	for _, e := range m.aclEntries {
		if e.aclType == ACLTypeUser && e.hasWritePermissions() {
			entries = append(entries, e)
		}
	}
	return entries
}

func (m *aclManager) getGroups() []*aclEntry {
	entries := []*aclEntry{}
	for _, e := range m.aclEntries {
		if e.aclType == ACLTypeGroup {
			entries = append(entries, e)
		}
	}
	return entries
}

func (m *aclManager) getGroupsWithReadPermission() []*aclEntry {
	entries := []*aclEntry{}
	for _, e := range m.aclEntries {
		if e.aclType == ACLTypeGroup && e.hasReadPermissions() {
			entries = append(entries, e)
		}
	}
	return entries
}

func (m *aclManager) getGroupsWithWritePermission() []*aclEntry {
	entries := []*aclEntry{}
	for _, e := range m.aclEntries {
		if e.aclType == ACLTypeGroup && e.hasWritePermissions() {
			entries = append(entries, e)
		}
	}
	return entries
}

func (m *aclManager) getUnixGroups() []*aclEntry {
	entries := []*aclEntry{}
	for _, e := range m.aclEntries {
		if e.aclType == ACLTypeUnixGroup {
			entries = append(entries, e)
		}
	}
	return entries
}

func (m *aclManager) getUnixGroupsWithReadPermission() []*aclEntry {
	entries := []*aclEntry{}
	for _, e := range m.aclEntries {
		if e.aclType == ACLTypeUnixGroup && e.hasReadPermissions() {
			entries = append(entries, e)
		}
	}
	return entries
}

func (m *aclManager) getUnixGroupsWithWritePermission() []*aclEntry {
	entries := []*aclEntry{}
	for _, e := range m.aclEntries {
		if e.aclType == ACLTypeUnixGroup && e.hasWritePermissions() {
			entries = append(entries, e)
		}
	}
	return entries
}

func (m *aclManager) getUser(username string) *aclEntry {
	for _, u := range m.getUsers() {
		if u.recipient == username {
			return u
		}
	}
	return nil
}

func (m *aclManager) getGroup(group string) *aclEntry {
	for _, e := range m.getGroups() {
		if e.recipient == group {
			return e
		}
	}
	return nil
}

func (m *aclManager) getUnixGroup(unixGroup string) *aclEntry {
	for _, e := range m.getUnixGroups() {
		if e.recipient == unixGroup {
			return e
		}
	}
	return nil
}

func (m *aclManager) deleteUser(ctx context.Context, username string) {
	for i, e := range m.aclEntries {
		if e.recipient == username && e.aclType == ACLTypeUser {
			m.aclEntries = append(m.aclEntries[:i], m.aclEntries[i+1:]...)
			return
		}
	}
}

func (m *aclManager) addUser(ctx context.Context, username string, mode ACLMode) error {
	m.deleteUser(ctx, username)
	sysACL := strings.Join([]string{string(ACLTypeUser), username, string(mode)}, ":")
	newEntry, err := newACLEntry(ctx, sysACL)
	if err != nil {
		return err
	}
	m.aclEntries = append(m.aclEntries, newEntry)
	return nil
}

func (m *aclManager) deleteGroup(ctx context.Context, group string) {
	for i, e := range m.aclEntries {
		if e.recipient == group && e.aclType == ACLTypeGroup {
			m.aclEntries = append(m.aclEntries[:i], m.aclEntries[i+1:]...)
			return
		}
	}
}

func (m *aclManager) addGroup(ctx context.Context, group string, mode ACLMode) error {
	m.deleteGroup(ctx, group)
	sysACL := strings.Join([]string{string(ACLTypeGroup), group, string(mode)}, ":")
	newEntry, err := newACLEntry(ctx, sysACL)
	if err != nil {
		return err
	}
	m.aclEntries = append(m.aclEntries, newEntry)
	return nil
}

func (m *aclManager) deleteUnixGroup(ctx context.Context, unixGroup string) {
	for i, e := range m.aclEntries {
		if e.recipient == unixGroup && e.aclType == ACLTypeUnixGroup {
			m.aclEntries = append(m.aclEntries[:i], m.aclEntries[i+1:]...)
			return
		}
	}
}

func (m *aclManager) addUnixGroup(ctx context.Context, unixGroup string, mode ACLMode) error {
	m.deleteUnixGroup(ctx, unixGroup)
	sysACL := strings.Join([]string{string(ACLTypeUnixGroup), unixGroup, string(mode)}, ":")
	newEntry, err := newACLEntry(ctx, sysACL)
	if err != nil {
		return err
	}
	m.aclEntries = append(m.aclEntries, newEntry)
	return nil
}
*/

func (m *aclManager) readOnlyToEOSPermissions(readOnly bool) string {
	if readOnly {
		return "rx"
	}
	return "rwx+d"
}

func (m *aclManager) serialize() string {
	sysACL := []string{}
	for _, e := range m.aclEntries {
		sysACL = append(sysACL, e.serialize())
	}
	return strings.Join(sysACL, ",")
}

type aclEntry struct {
	aclType   string
	recipient string
	mode      string
}

// u:gonzalhu:rw
func newACLEntry(ctx context.Context, singleSysACL string) (*aclEntry, error) {
	tokens := strings.Split(singleSysACL, ":")
	if len(tokens) != 3 {
		return nil, errInvalidACL
	}

	aclType := tokens[0]
	target := tokens[1]
	mode := tokens[2]

	return &aclEntry{
		aclType:   aclType,
		recipient: target,
		mode:      mode,
	}, nil
}

/*
func (a *aclEntry) hasWritePermissions() bool {
	return a.mode == ACLModeReadWrite
}

func (a *aclEntry) hasReadPermissions() bool {
	return a.mode == ACLModeRead || a.mode == ACLModeReadWrite
}
*/

func (a *aclEntry) serialize() string {
	return strings.Join([]string{string(a.aclType), a.recipient, a.mode}, ":")
}

type notFoundError string

func (e notFoundError) IsNotFound()   {}
func (e notFoundError) Error() string { return string(e) }
