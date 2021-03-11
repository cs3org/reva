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

package eosgrpc

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

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/eosclient"
	erpc "github.com/cs3org/reva/pkg/eosclient/eosgrpc/eos_grpc"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/acl"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/cs3org/reva/pkg/logger"
)

const (
	versionPrefix = ".sys.v#."
)

const (
	// SystemAttr is the system extended attribute.
	SystemAttr eosclient.AttrType = iota
	// UserAttr is the user extended attribute.
	UserAttr
)

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

	// Location of the xrdcopy binary.
	// Default is /opt/eos/xrootd/bin/xrdcopy.
	XrdcopyBinary string

	// URL of the EOS MGM.
	// Default is root://eos-example.org
	URL string

	// URI of the EOS MGM grpc server
	GrpcURI string

	// Location on the local fs where to store reads.
	// Defaults to os.TempDir()
	CacheDirectory string

	// Keytab is the location of the EOS keytab file.
	Keytab string

	// Authkey is the key that authorizes this client to connect to the GRPC service
	// It's unclear whether this will be the final solution
	Authkey string

	// SecProtocol is the comma separated list of security protocols used by xrootd.
	// For example: "sss, unix"
	SecProtocol string
}

func (opt *Options) init() {
	if opt.ForceSingleUserMode && opt.SingleUsername != "" {
		opt.SingleUsername = "apache"
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

// Client performs actions against a EOS management node (MGM)
// using the EOS GRPC interface.
type Client struct {
	opt *Options
	cl  erpc.EosClient
}

// Create and connect a grpc eos Client
func newgrpc(ctx context.Context, opt *Options) (erpc.EosClient, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("Connecting to ", "'"+opt.GrpcURI+"'").Msg("")

	conn, err := grpc.Dial(opt.GrpcURI, grpc.WithInsecure())
	if err != nil {
		log.Debug().Str("Error connecting to ", "'"+opt.GrpcURI+"' ").Str("err:", err.Error()).Msg("")
		return nil, err
	}

	log.Debug().Str("Going to ping ", "'"+opt.GrpcURI+"' ").Msg("")
	ecl := erpc.NewEosClient(conn)
	// If we can't ping... treat it as an error... we will see if this has to be kept, for now it's practical
	prq := new(erpc.PingRequest)
	prq.Authkey = opt.Authkey
	prq.Message = []byte("hi this is a ping from reva")
	prep, err := ecl.Ping(ctx, prq)
	if err != nil {
		log.Error().Str("Ping to ", "'"+opt.GrpcURI+"' ").Str("err:", err.Error()).Msg("")
		return nil, err
	}

	if prep == nil {
		log.Debug().Str("Ping to ", "'"+opt.GrpcURI+"' ").Str("gave nil response", "").Msg("")
		return nil, errtypes.InternalError("nil response from ping")
	}

	log.Info().Str("Ping to ", "'"+opt.GrpcURI+"' ").Msg(" was successful")
	return ecl, nil
}

// New creates a new client with the given options.
func New(opt *Options) *Client {
	tlog := logger.New().With().Int("pid", os.Getpid()).Logger()
	tlog.Debug().Str("Creating new eosgrpc client. opt: ", "'"+fmt.Sprintf("%#v", opt)+"' ").Msg("")

	opt.init()
	c := new(Client)
	c.opt = opt

	// Let's be successful if the ping was ok. This is an initialization phase
	// and we enforce the server to be up
	// This will likely improve as soon as the behaviour of grpc is understood
	// in the case of server restarts or failures
	tctx := appctx.WithLogger(context.Background(), &tlog)
	ccl, err := newgrpc(tctx, opt)
	if err != nil {
		return nil
	}
	c.cl = ccl

	return c
}

// Common code to create and initialize a NSRequest
func (c *Client) initNSRequest(uid, gid string) (*erpc.NSRequest, error) {
	// Stuff filename, uid, gid into the MDRequest type
	rq := new(erpc.NSRequest)
	rq.Role = new(erpc.RoleId)

	uidInt, err := strconv.ParseUint(uid, 10, 64)
	if err != nil {
		return nil, err
	}
	gidInt, err := strconv.ParseUint(gid, 10, 64)
	if err != nil {
		return nil, err
	}
	rq.Role.Uid = uidInt
	rq.Role.Gid = gidInt
	rq.Authkey = c.opt.Authkey

	return rq, nil
}

// Common code to create and initialize a NSRequest
func (c *Client) initMDRequest(uid, gid string) (*erpc.MDRequest, error) {
	// Stuff filename, uid, gid into the MDRequest type
	mdrq := new(erpc.MDRequest)
	mdrq.Role = new(erpc.RoleId)

	uidInt, err := strconv.ParseUint(uid, 10, 64)
	if err != nil {
		return nil, err
	}
	gidInt, err := strconv.ParseUint(gid, 10, 64)
	if err != nil {
		return nil, err
	}
	mdrq.Role.Uid = uidInt
	mdrq.Role.Gid = gidInt

	mdrq.Authkey = c.opt.Authkey

	return mdrq, nil
}

// AddACL adds an new acl to EOS with the given aclType.
func (c *Client) AddACL(ctx context.Context, uid, gid, rootUID, rootGID, path string, a *acl.Entry) error {
	log := appctx.GetLogger(ctx)

	acls, err := c.getACLForPath(ctx, uid, gid, path)
	if err != nil {
		return err
	}

	err = acls.SetEntry(a.Type, a.Qualifier, a.Permissions)
	if err != nil {
		return err
	}
	sysACL := acls.Serialize()

	// Init a new NSRequest
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_AclRequest)
	msg.Cmd = erpc.NSRequest_AclRequest_ACL_COMMAND(erpc.NSRequest_AclRequest_ACL_COMMAND_value["MODIFY"])
	msg.Type = erpc.NSRequest_AclRequest_ACL_TYPE(erpc.NSRequest_AclRequest_ACL_TYPE_value["SYS_ACL"])
	msg.Recursive = true
	msg.Rule = sysACL

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Acl{Acl: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(context.Background(), rq)
	if err != nil {
		log.Error().Str("Exec ", "'"+path+"' ").Str("err:", err.Error()).Msg("")
		return err
	}

	log.Debug().Str("Exec ", "'"+path+"' ").Str("resp:", fmt.Sprintf("%#v", resp)).Msg("")
	if resp == nil {
		return errtypes.NotFound(fmt.Sprintf("Path: %s", path))
	}

	return err

}

// RemoveACL removes the acl from EOS.
func (c *Client) RemoveACL(ctx context.Context, uid, gid, rootUID, rootGID, path string, a *acl.Entry) error {
	log := appctx.GetLogger(ctx)

	acls, err := c.getACLForPath(ctx, uid, gid, path)
	if err != nil {
		return err
	}

	acls.DeleteEntry(a.Type, a.Qualifier)
	sysACL := acls.Serialize()

	// Init a new NSRequest
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_AclRequest)
	msg.Cmd = erpc.NSRequest_AclRequest_ACL_COMMAND(erpc.NSRequest_AclRequest_ACL_COMMAND_value["MODIFY"])
	msg.Type = erpc.NSRequest_AclRequest_ACL_TYPE(erpc.NSRequest_AclRequest_ACL_TYPE_value["SYS_ACL"])
	msg.Recursive = true
	msg.Rule = sysACL

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Acl{Acl: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(context.Background(), rq)
	if err != nil {
		log.Error().Str("Exec ", "'"+path+"' ").Str("err:", err.Error()).Msg("")
		return err
	}

	log.Debug().Str("Exec ", "'"+path+"' ").Str("resp:", fmt.Sprintf("%#v", resp)).Msg("")
	if resp == nil {
		return errtypes.NotFound(fmt.Sprintf("Path: %s", path))
	}

	return err

}

// UpdateACL updates the EOS acl.
func (c *Client) UpdateACL(ctx context.Context, uid, gid, rootUID, rootGID, path string, a *acl.Entry) error {
	return c.AddACL(ctx, uid, gid, path, rootUID, rootGID, a)
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
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return nil, err
	}

	msg := new(erpc.NSRequest_AclRequest)
	msg.Cmd = erpc.NSRequest_AclRequest_ACL_COMMAND(erpc.NSRequest_AclRequest_ACL_COMMAND_value["LIST"])
	msg.Type = erpc.NSRequest_AclRequest_ACL_TYPE(erpc.NSRequest_AclRequest_ACL_TYPE_value["SYS_ACL"])
	msg.Recursive = true

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Acl{Acl: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(context.Background(), rq)

	if err != nil {
		log.Error().Err(err).Str("path", path).Str("err", err.Error())
		return nil, err
	}

	if resp == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' path: '%s'", uid, path))
	}

	log.Debug().Str("Exec ", "'"+path+"' ").Str("resp:", fmt.Sprintf("%#v", resp)).Msg("")

	if resp.Acl == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil acl for uid: '%s' path: '%s'", uid, path))
	}

	if resp.GetError() != nil {
		log.Info().Str("uid", uid).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")
	}

	aclret, err := acl.Parse(resp.Acl.Rule, acl.ShortTextForm)

	// Now loop and build the correct return value

	return aclret, err
}

// GetFileInfoByInode returns the FileInfo by the given inode
func (c *Client) GetFileInfoByInode(ctx context.Context, uid, gid string, inode uint64) (*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the MDReq
	mdrq, err := c.initMDRequest(uid, gid)
	if err != nil {
		return nil, err
	}

	// Stuff filename, uid, gid into the MDRequest type
	mdrq.Type = erpc.TYPE_STAT
	mdrq.Id = new(erpc.MDId)
	mdrq.Id.Ino = inode

	// Now send the req and see what happens
	resp, err := c.cl.MD(context.Background(), mdrq)
	if err != nil {
		log.Error().Err(err).Uint64("inode", inode).Str("err", err.Error())

		return nil, err
	}
	rsp, err := resp.Recv()
	if err != nil {
		log.Error().Err(err).Uint64("inode", inode).Str("err", err.Error())
		return nil, err
	}

	if rsp == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil response for inode: '%d'", inode))
	}

	log.Info().Uint64("inode", inode).Str("rsp:", fmt.Sprintf("%#v", rsp)).Msg("grpc response")

	info, err := c.grpcMDResponseToFileInfo(rsp, "")
	if err != nil {
		return nil, err
	}

	if c.opt.VersionInvariant && isVersionFolder(info.File) {
		info, err = c.getFileInfoFromVersion(ctx, uid, gid, info.File)
		if err != nil {
			return nil, err
		}
		info.Inode = inode
	}

	return info, nil
}

// SetAttr sets an extended attributes on a path.
func (c *Client) SetAttr(ctx context.Context, uid, gid string, attr *eosclient.Attribute, recursive bool, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_SetXAttrRequest)

	var m = map[string][]byte{attr.Key: []byte(attr.Val)}
	msg.Xattrs = m
	msg.Recursive = recursive

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Xattr{Xattr: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' gid: '%s' path: '%s'", uid, gid, path))
	}

	log.Info().Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	return err

}

// UnsetAttr unsets an extended attribute on a path.
func (c *Client) UnsetAttr(ctx context.Context, uid, gid string, attr *eosclient.Attribute, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_SetXAttrRequest)

	var ktd = []string{attr.Key}
	msg.Keystodelete = ktd

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Xattr{Xattr: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(ctx, rq)
	if err != nil {
		log.Error().Err(err).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' gid: '%s' path: '%s'", uid, gid, path))
	}

	log.Info().Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	return err

}

// GetFileInfoByPath returns the FilInfo at the given path
func (c *Client) GetFileInfoByPath(ctx context.Context, uid, gid, path string) (*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the MDReq
	mdrq, err := c.initMDRequest(uid, gid)
	if err != nil {
		return nil, err
	}

	mdrq.Type = erpc.TYPE_STAT
	mdrq.Id = new(erpc.MDId)
	mdrq.Id.Path = []byte(path)

	// Now send the req and see what happens
	resp, err := c.cl.MD(ctx, mdrq)
	if err != nil {
		log.Error().Err(err).Str("path", path).Str("err", err.Error())

		return nil, err
	}
	rsp, err := resp.Recv()
	if err != nil {
		log.Error().Err(err).Str("path", path).Str("err", err.Error())

		// FIXME: this is very very bad and poisonous for the project!!!!!!!
		// Apparently here we have to assume that an error in Recv() means "file not found"
		// - "File not found is not an error", it's a legitimate result of a legitimate check
		// - Assuming that any error means file not found is doubly poisonous
		return nil, errtypes.NotFound(err.Error())
		// return nil, nil
	}

	if rsp == nil {
		return nil, errtypes.NotFound(fmt.Sprintf("%s:%s", "acltype", path))
	}

	log.Info().Str("path", path).Str("rsp:", fmt.Sprintf("%#v", rsp)).Msg("grpc response")

	info, err := c.grpcMDResponseToFileInfo(rsp, filepath.Dir(path))
	if err != nil {
		return nil, err
	}

	if c.opt.VersionInvariant && !isVersionFolder(path) && !info.IsDir {
		inode, err := c.getVersionFolderInode(ctx, uid, gid, path)
		if err != nil {
			return nil, err
		}
		info.Inode = inode
	}
	return info, nil
}

// GetFileInfoByFXID returns the FileInfo by the given file id in hexadecimal
func (c *Client) GetFileInfoByFXID(ctx context.Context, uid, gid string, fxid string) (*eosclient.FileInfo, error) {
	return nil, errtypes.NotSupported("eosgrpc: GetFileInfoByFXID not implemented")
}

// GetQuota gets the quota of a user on the quota node defined by path
func (c *Client) GetQuota(ctx context.Context, username, rootUID, rootGID, path string) (*eosclient.QuotaInfo, error) {
	return nil, errtypes.NotSupported("eosgrpc: GetQuota not implemented")
}

// SetQuota sets the quota of a user on the quota node defined by path
func (c *Client) SetQuota(ctx context.Context, rootUID, rootGID string, info *eosclient.SetQuotaInfo) error {
	return errtypes.NotSupported("eosgrpc: SetQuota not implemented")
}

// Touch creates a 0-size,0-replica file in the EOS namespace.
func (c *Client) Touch(ctx context.Context, uid, gid, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_TouchRequest)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Touch{Touch: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' path: '%s'", uid, path))
	}

	log.Info().Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err

}

// Chown given path
func (c *Client) Chown(ctx context.Context, uid, gid, chownUID, chownGID, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_ChownRequest)
	msg.Owner = new(erpc.RoleId)
	msg.Owner.Uid, err = strconv.ParseUint(chownUID, 10, 64)
	if err != nil {
		return err
	}

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Chown{Chown: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(ctx, rq)
	if err != nil {
		log.Error().Err(err).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' chownuid: '%s' path: '%s'", uid, chownUID, path))
	}

	log.Info().Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err

}

// Chmod given path
func (c *Client) Chmod(ctx context.Context, uid, gid, mode, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_ChmodRequest)

	md, err := strconv.ParseUint(mode, 8, 64)
	if err != nil {
		return err
	}
	msg.Mode = int64(md)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Chmod{Chmod: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("mode", mode).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' mode: '%s' path: '%s'", uid, mode, path))
	}

	log.Info().Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err

}

// CreateDir creates a directory at the given path
func (c *Client) CreateDir(ctx context.Context, uid, gid, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_MkdirRequest)

	// Let's put 750 as permissions, assuming that EOS will apply some mask
	md, err := strconv.ParseUint("750", 8, 64)
	if err != nil {
		return err
	}
	msg.Mode = int64(md)
	msg.Recursive = true
	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Mkdir{Mkdir: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' path: '%s'", uid, path))
	}

	log.Info().Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err

}

func (c *Client) rm(ctx context.Context, uid, gid, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_UnlinkRequest)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Unlink{Unlink: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' path: '%s'", uid, path))
	}

	log.Info().Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err

}

func (c *Client) rmdir(ctx context.Context, uid, gid, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_RmRequest)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)
	msg.Recursive = true
	msg.Norecycle = false

	rq.Command = &erpc.NSRequest_Rm{Rm: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' path: '%s'", uid, path))
	}

	log.Info().Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err
}

// Remove removes the resource at the given path
func (c *Client) Remove(ctx context.Context, uid, gid, path string) error {
	log := appctx.GetLogger(ctx)

	nfo, err := c.GetFileInfoByPath(ctx, uid, gid, path)
	if err != nil {
		log.Warn().Err(err).Str("path", path).Str("err", err.Error())
		return err
	}

	if nfo.IsDir {
		return c.rmdir(ctx, uid, gid, path)
	}

	return c.rm(ctx, uid, gid, path)
}

// Rename renames the resource referenced by oldPath to newPath
func (c *Client) Rename(ctx context.Context, uid, gid, oldPath, newPath string) error {
	return errtypes.NotSupported("eosgrpc: Rename not implemented")
}

// List the contents of the directory given by path
func (c *Client) List(ctx context.Context, uid, gid, dpath string) ([]*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)

	// Stuff filename, uid, gid into the FindRequest type
	fdrq := new(erpc.FindRequest)
	fdrq.Maxdepth = 1
	fdrq.Type = erpc.TYPE_LISTING
	fdrq.Id = new(erpc.MDId)
	fdrq.Id.Path = []byte(dpath)

	fdrq.Role = new(erpc.RoleId)

	uidInt, err := strconv.ParseUint(uid, 10, 64)
	if err != nil {
		return nil, err
	}
	gidInt, err := strconv.ParseUint(gid, 10, 64)
	if err != nil {
		return nil, err
	}
	fdrq.Role.Uid = uidInt
	fdrq.Role.Gid = gidInt

	fdrq.Authkey = c.opt.Authkey

	// Now send the req and see what happens
	resp, err := c.cl.Find(context.Background(), fdrq)
	if err != nil {
		log.Error().Err(err).Str("path", dpath).Str("err", err.Error())

		return nil, err
	}

	var mylst []*eosclient.FileInfo
	i := 0
	for {
		rsp, err := resp.Recv()
		if err != nil {
			if err == io.EOF {
				return mylst, nil
			}

			log.Warn().Err(err).Str("path", dpath).Str("err", err.Error())

			return nil, err
		}

		if rsp == nil {
			log.Warn().Err(err).Str("path", dpath).Str("err", "rsp is nil")
			return nil, errtypes.NotFound(dpath)
		}

		log.Debug().Str("path", dpath).Str("item resp:", fmt.Sprintf("%#v", rsp)).Msg("grpc response")

		myitem, err := c.grpcMDResponseToFileInfo(rsp, dpath)
		if err != nil {
			log.Warn().Err(err).Str("path", dpath).Str("could not convert item:", fmt.Sprintf("%#v", rsp)).Str("err:", err.Error()).Msg("")

			return nil, err
		}

		i++
		if i == 1 {
			continue
		}
		mylst = append(mylst, myitem)
	}
}

// Read reads a file from the mgm
func (c *Client) Read(ctx context.Context, uid, gid, path string) (io.ReadCloser, error) {
	rand := "eosread-" + uuid.New().String()
	localTarget := fmt.Sprintf("%s/%s", c.opt.CacheDirectory, rand)
	defer os.RemoveAll(localTarget)

	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	cmd := exec.CommandContext(ctx, c.opt.XrdcopyBinary, "--nopbar", "--silent", "-f", xrdPath, localTarget, fmt.Sprintf("-OSeos.ruid=%s&eos.rgid=%s", uid, gid))
	if _, _, err := c.execute(ctx, cmd); err != nil {
		return nil, err
	}
	return os.Open(localTarget)
}

// Write writes a file to the mgm
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
func (c *Client) ListDeletedEntries(ctx context.Context, uid, gid string) ([]*eosclient.DeletedEntry, error) {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return nil, err
	}

	msg := new(erpc.NSRequest_RecycleRequest)
	msg.Cmd = erpc.NSRequest_RecycleRequest_RECYCLE_CMD(erpc.NSRequest_RecycleRequest_RECYCLE_CMD_value["LIST"])

	rq.Command = &erpc.NSRequest_Recycle{Recycle: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(context.Background(), rq)
	if err != nil {
		log.Warn().Err(err).Str("err", err.Error())
		return nil, err
	}

	if resp == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s'", uid))
	}

	log.Info().Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	// TODO(labkode): add protection if slave is configured and alive to count how many files are in the trashbin before
	// triggering the recycle ls call that could break the instance because of unavailable memory.
	// FF: I agree with labkode, if we think we may have memory problems then the semantics of the grpc call`and
	// the semantics if this func will have to change. For now this is not foreseen

	ret := make([]*eosclient.DeletedEntry, 0)
	for _, f := range resp.Recycle.Recycles {
		if f == nil {
			log.Info().Msg("nil item in response")
			continue
		}

		entry := &eosclient.DeletedEntry{
			RestorePath:   string(f.Id.Path),
			RestoreKey:    f.Key,
			Size:          f.Size,
			DeletionMTime: f.Dtime.Sec,
			IsDir:         (f.Type == erpc.NSResponse_RecycleResponse_RecycleInfo_TREE),
		}

		ret = append(ret, entry)
	}

	return ret, nil
}

// RestoreDeletedEntry restores a deleted entry.
func (c *Client) RestoreDeletedEntry(ctx context.Context, uid, gid, key string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_RecycleRequest)
	msg.Cmd = erpc.NSRequest_RecycleRequest_RECYCLE_CMD(erpc.NSRequest_RecycleRequest_RECYCLE_CMD_value["RESTORE"])

	msg.Key = key

	rq.Command = &erpc.NSRequest_Recycle{Recycle: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(context.Background(), rq)
	if err != nil {
		log.Warn().Err(err).Str("key", key).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' key: '%s'", uid, key))
	}

	log.Info().Str("key", key).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	return err
}

// PurgeDeletedEntries purges all entries from the recycle bin.
func (c *Client) PurgeDeletedEntries(ctx context.Context, uid, gid string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(uid, gid)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_RecycleRequest)
	msg.Cmd = erpc.NSRequest_RecycleRequest_RECYCLE_CMD(erpc.NSRequest_RecycleRequest_RECYCLE_CMD_value["PURGE"])

	rq.Command = &erpc.NSRequest_Recycle{Recycle: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(context.Background(), rq)
	if err != nil {
		log.Warn().Err(err).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' ", uid))
	}

	log.Info().Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	return err
}

// ListVersions list all the versions for a given file.
func (c *Client) ListVersions(ctx context.Context, uid, gid, p string) ([]*eosclient.FileInfo, error) {
	versionFolder := getVersionFolder(p)
	finfos, err := c.List(ctx, uid, gid, versionFolder)
	if err != nil {
		// we send back an empty list
		return []*eosclient.FileInfo{}, nil
	}
	return finfos, nil
}

// RollbackToVersion rollbacks a file to a previous version.
func (c *Client) RollbackToVersion(ctx context.Context, uid, gid, path, version string) error {
	// TODO(ffurano):
	/*
		cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", uid, gid, "file", "versions", path, version)
		_, _, err = c.executeEOS(ctx, cmd)
		return err
	*/
	return errtypes.NotSupported("TODO")
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

func (c *Client) getFileInfoFromVersion(ctx context.Context, uid, gid, p string) (*eosclient.FileInfo, error) {
	file := getFileFromVersionFolder(p)
	md, err := c.GetFileInfoByPath(ctx, uid, gid, file)
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

func (c *Client) grpcMDResponseToFileInfo(st *erpc.MDResponse, namepfx string) (*eosclient.FileInfo, error) {
	if st.Cmd == nil && st.Fmd == nil {
		return nil, errors.Wrap(errtypes.NotSupported(""), "Invalid response (st.Cmd and st.Fmd are nil)")
	}
	fi := new(eosclient.FileInfo)

	if st.Type != 0 {
		fi.IsDir = true
	}
	if st.Fmd != nil {
		fi.Inode = st.Fmd.Id
		fi.UID = st.Fmd.Uid
		fi.GID = st.Fmd.Gid
		fi.MTimeSec = st.Fmd.Mtime.Sec
		fi.ETag = st.Fmd.Etag
		if namepfx == "" {
			fi.File = string(st.Fmd.Name)
		} else {
			fi.File = namepfx + "/" + string(st.Fmd.Name)
		}

		for k, v := range st.Fmd.Xattrs {
			if fi.Attrs == nil {
				fi.Attrs = make(map[string]string)
			}
			fi.Attrs[k] = string(v)
		}

		fi.Size = st.Fmd.Size

	} else {
		fi.Inode = st.Cmd.Id
		fi.UID = st.Cmd.Uid
		fi.GID = st.Cmd.Gid
		fi.MTimeSec = st.Cmd.Mtime.Sec
		fi.ETag = st.Cmd.Etag
		if namepfx == "" {
			fi.File = string(st.Cmd.Name)
		} else {
			fi.File = namepfx + "/" + string(st.Cmd.Name)
		}

		var allattrs = ""
		for k, v := range st.Cmd.Xattrs {
			if fi.Attrs == nil {
				fi.Attrs = make(map[string]string)
			}
			fi.Attrs[k] = string(v)
			allattrs += string(v)
			allattrs += ","
		}

		fi.Size = 0
	}

	log.Debug().Str("stat info - path", fi.File).Uint64("inode:", fi.Inode).Uint64("uid:", fi.UID).Uint64("gid:", fi.GID).Str("etag:", fi.ETag).Msg("grpc response")

	return fi, nil
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
