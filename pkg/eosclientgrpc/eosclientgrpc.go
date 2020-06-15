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

package eosclientgrpc

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
	"syscall"

	"github.com/cs3org/reva/pkg/appctx"
	erpc "github.com/cs3org/reva/pkg/eosclientgrpc/eos_grpc"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/acl"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/cs3org/reva/pkg/logger"
)

const (
	versionPrefix = ".sys.v#."
)

// AttrType is the type of extended attribute,
// either system (sys) or user (user).
type AttrType uint32

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

// Options to configure the Client.
type Options struct {

	// ForceSingleUserMode forces all connections to use only one user.
	// This is the case when access to EOS is done from FUSE under apache or www-data.
	ForceSingleUserMode bool

	// UseKeyTabAuth changes will authenticate requests by using an EOS keytab.
	UseKeytab bool

	// SingleUsername is the username to use when connecting to EOS.
	// Defaults to apache
	SingleUsername string

	// Location of the xrdcopy binary.
	// Default is /usr/bin/xrdcopy.
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
		opt.XrdcopyBinary = "/usr/bin/xrdcopy"
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

	tctx := appctx.WithLogger(context.Background(), &tlog)

	// Let's be successful if the ping was ok. This is an initialization phase
	// and we enforce the server to be up
	// This will likely improve as soon as the behaviour of grpc is understood
	// in the case of server restarts or failures
	ccl, err := newgrpc(tctx, opt)
	if err != nil {
		return nil
	}
	c.cl = ccl

	// Some connection tests, useful for logging in this dev phase
	tlog.Debug().Str("Connection tests to: ", "'"+opt.GrpcURI+"' ").Msg("")

	tlog.Debug().Str("Going to stat", "/eos").Msg("")
	frep, err := c.GetFileInfoByPath(tctx, "furano", "/eos")
	if err != nil {
		tlog.Error().Str("GetFileInfoByPath /eos to ", "'"+opt.GrpcURI+"' ").Str("err:", err.Error()).Msg("")
		//	return nil
	} else {
		tlog.Info().Str("GetFileInfoByPath /eos to ", "'"+opt.GrpcURI+"' ").Str("resp:", frep.File).Msg("")
	}

	tlog.Debug().Str("Going to stat", "/eos-idonotexist").Msg("")
	frep1, err := c.GetFileInfoByPath(tctx, "furano", "/eos-idonotexist")
	if err != nil {
		tlog.Info().Str("GetFileInfoByPath /eos-idonotexist to ", "'"+opt.GrpcURI+"' ").Str("err:", err.Error()).Msg("")

		//	return nil
	} else {
		tlog.Error().Str("GetFileInfoByPath /eos-idonotexist to ", "'"+opt.GrpcURI+"' ").Str("wrong resp:", frep1.File).Msg("")
	}

	tlog.Debug().Str("Going to list", "/eos").Msg("")
	lrep, err := c.List(context.Background(), "furano", "/eos")
	if err != nil {
		tlog.Error().Str("List /eos to ", "'"+opt.GrpcURI+"' ").Str("err:", err.Error()).Msg("")
		//	return nil
	} else {
		tlog.Info().Str("List /eos to ", "'"+opt.GrpcURI+"' ").Int("nentries:", len(lrep)).Msg("")
	}

	return c
}

// Common code to create and initialize a NSRequest
func (c *Client) initNSRequest(username string) (*erpc.NSRequest, error) {
	// Stuff filename, uid, gid into the MDRequest type
	rq := new(erpc.NSRequest)

	// setting of the sys.acl is only possible from root user
	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return nil, err
	}
	rq.Role = new(erpc.RoleId)

	uid, err := strconv.ParseUint(unixUser.Uid, 10, 64)
	if err != nil {
		return nil, err
	}
	rq.Role.Uid = uid
	gid, err := strconv.ParseUint(unixUser.Gid, 10, 64)
	if err != nil {
		return nil, err
	}
	rq.Role.Gid = gid

	rq.Authkey = c.opt.Authkey

	return rq, nil
}

// Common code to create and initialize a NSRequest
func (c *Client) initMDRequest(username string) (*erpc.MDRequest, error) {
	// Stuff filename, uid, gid into the MDRequest type
	mdrq := new(erpc.MDRequest)

	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return nil, err
	}
	mdrq.Role = new(erpc.RoleId)

	uid, err := strconv.ParseUint(unixUser.Uid, 10, 64)
	if err != nil {
		return nil, err
	}
	mdrq.Role.Uid = uid
	gid, err := strconv.ParseUint(unixUser.Gid, 10, 64)
	if err != nil {
		return nil, err
	}
	mdrq.Role.Gid = gid

	mdrq.Authkey = c.opt.Authkey

	return mdrq, nil
}

func (c *Client) getUnixUser(username string) (*gouser.User, error) {
	if c.opt.ForceSingleUserMode {
		username = c.opt.SingleUsername
	}
	return gouser.Lookup(username)
}

// AddACL adds an new acl to EOS with the given aclType.
func (c *Client) AddACL(ctx context.Context, username, path string, a *acl.Entry) error {
	log := appctx.GetLogger(ctx)

	acls, err := c.getACLForPath(ctx, username, path)
	if err != nil {
		return err
	}

	// since EOS Citrine ACLs are is stored with uid, we need to convert username to uid
	// only for users.
	if a.Type == acl.TypeUser {
		a.Qualifier, err = getUID(a.Qualifier)
		if err != nil {
			return err
		}
	}
	err = acls.SetEntry(a.Type, a.Qualifier, a.Permissions)
	if err != nil {
		return err
	}
	sysACL := acls.Serialize()

	// Init a new NSRequest
	rq, err := c.initNSRequest(username)
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
func (c *Client) RemoveACL(ctx context.Context, username, path string, aclType string, recipient string) error {
	log := appctx.GetLogger(ctx)

	acls, err := c.getACLForPath(ctx, username, path)
	if err != nil {
		return err
	}

	// since EOS Citrine ACLs are is stored with uid, we need to convert username to uid
	// only for users.

	// since EOS Citrine ACLs are stored with uid, we need to convert username to uid
	if aclType == acl.TypeUser {
		recipient, err = getUID(recipient)
		if err != nil {
			return err
		}
	}
	acls.DeleteEntry(aclType, recipient)
	sysACL := acls.Serialize()

	// Init a new NSRequest
	rq, err := c.initNSRequest(username)
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
func (c *Client) UpdateACL(ctx context.Context, username, path string, a *acl.Entry) error {
	return c.AddACL(ctx, username, path, a)
}

// GetACL for a file
func (c *Client) GetACL(ctx context.Context, username, path, aclType, target string) (*acl.Entry, error) {
	acls, err := c.ListACLs(ctx, username, path)
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

func getUsername(uid string) (string, error) {
	user, err := gouser.LookupId(uid)
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

func getUID(username string) (string, error) {
	user, err := gouser.Lookup(username)
	if err != nil {
		return "", err
	}
	return user.Uid, nil
}

// ListACLs returns the list of ACLs present under the given path.
// EOS returns uids/gid for Citrine version and usernames for older versions.
// For Citire we need to convert back the uid back to username.
func (c *Client) ListACLs(ctx context.Context, username, path string) ([]*acl.Entry, error) {
	log := appctx.GetLogger(ctx)

	parsedACLs, err := c.getACLForPath(ctx, username, path)
	if err != nil {
		return nil, err
	}

	acls := []*acl.Entry{}
	for _, acl := range parsedACLs.Entries {
		// since EOS Citrine ACLs are is stored with uid, we need to convert uid to userame
		// TODO map group names as well if acl.Type == "g" ...
		acl.Qualifier, err = getUsername(acl.Qualifier)
		if err != nil {
			log.Warn().Err(err).Str("path", path).Str("username", username).Str("qualifier", acl.Qualifier).Msg("cannot map qualifier to name")
			continue
		}
		acls = append(acls, acl)
	}
	return acls, nil
}

func (c *Client) getACLForPath(ctx context.Context, username, path string) (*acl.ACLs, error) {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
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
		log.Error().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return nil, err
	}

	if resp == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	log.Debug().Str("Exec ", "'"+path+"' ").Str("resp:", fmt.Sprintf("%#v", resp)).Msg("")

	if resp.Acl == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil acl for username: '%s' path: '%s'", username, path))
	}

	if resp.GetError() != nil {
		log.Info().Str("username", username).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")
	}

	aclret, err := acl.Parse(resp.Acl.Rule, acl.ShortTextForm)

	// Now loop and build the correct return value

	return aclret, err
}

// GetFileInfoByInode returns the FileInfo by the given inode
func (c *Client) GetFileInfoByInode(ctx context.Context, username string, inode uint64) (*FileInfo, error) {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the MDReq
	mdrq, err := c.initMDRequest(username)
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

	log.Info().Uint64("inode", inode).Msg("grpc response")

	return c.grpcMDResponseToFileInfo(rsp, "")
}

// SetAttr sets an extended attributes on a path.
func (c *Client) SetAttr(ctx context.Context, username string, attr *Attribute, recursive bool, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
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
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	log.Info().Str("username", username).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	return err

}

// UnsetAttr unsets an extended attribute on a path.
func (c *Client) UnsetAttr(ctx context.Context, username string, attr *Attribute, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
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
		log.Error().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	log.Info().Str("username", username).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	return err

}

// GetFileInfoByPath returns the FilInfo at the given path
func (c *Client) GetFileInfoByPath(ctx context.Context, username, path string) (*FileInfo, error) {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the MDReq
	mdrq, err := c.initMDRequest(username)
	if err != nil {
		return nil, err
	}

	mdrq.Type = erpc.TYPE_STAT
	mdrq.Id = new(erpc.MDId)
	mdrq.Id.Path = []byte(path)

	// Now send the req and see what happens
	resp, err := c.cl.MD(ctx, mdrq)
	if err != nil {
		log.Error().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())

		return nil, err
	}
	rsp, err := resp.Recv()
	if err != nil {
		log.Error().Err(err).Str("FIXME username", username).Str("path", path).Str("err", err.Error())

		// FIXME: this is very very bad and poisonous for the project!!!!!!!
		// Apparently here we have to assume that an error in Recv() means "file not found"
		// - "File not found is not an error", it's a legitimate result of a legitimate check
		// - Assuming that any error means file not found is doubly poisonous
		return nil, errtypes.NotFound(err.Error())
		//return nil, nil
	}

	if rsp == nil {
		return nil, errtypes.NotFound(fmt.Sprintf("%s:%s", "acltype", path))
	}

	log.Info().Str("username", username).Str("path", path).Str("rsp:", fmt.Sprintf("%#v", rsp)).Msg("grpc response")

	return c.grpcMDResponseToFileInfo(rsp, "")

}

// GetQuota gets the quota of a user on the quota node defined by path
func (c *Client) GetQuota(ctx context.Context, username, path string) (int, int, error) {
	return 0, 0, errtypes.NotSupported(fmt.Sprintf("%s:%s", "acltype", path))
}

// Touch creates a 0-size,0-replica file in the EOS namespace.
func (c *Client) Touch(ctx context.Context, username, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
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
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	log.Info().Str("username", username).Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err

}

// Chown given path
func (c *Client) Chown(ctx context.Context, username, chownUser, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_ChownRequest)
	msg.Owner = new(erpc.RoleId)

	chownunixUser, err := c.getUnixUser(chownUser)
	if err != nil {
		return err
	}

	msg.Owner.Uid, err = strconv.ParseUint(chownunixUser.Uid, 10, 64)
	if err != nil {
		return err
	}

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Chown{Chown: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(ctx, rq)
	if err != nil {
		log.Error().Err(err).Str("username", username).Str("chownuser", chownUser).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' chownuser: '%s' path: '%s'", username, chownUser, path))
	}

	log.Info().Str("username", username).Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err

}

// Chmod given path
func (c *Client) Chmod(ctx context.Context, username, mode, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
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
		log.Warn().Err(err).Str("username", username).Str("mode", mode).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' mode: '%s' path: '%s'", username, mode, path))
	}

	log.Info().Str("username", username).Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err

}

// CreateDir creates a directory at the given path
func (c *Client) CreateDir(ctx context.Context, username, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
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
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	log.Info().Str("username", username).Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err

}

func (c *Client) rm(ctx context.Context, username, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
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
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	log.Info().Str("username", username).Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err

}

func (c *Client) rmdir(ctx context.Context, username, path string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_RmdirRequest)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Rmdir{Rmdir: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(ctx, rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' path: '%s'", username, path))
	}

	log.Info().Str("username", username).Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err
}

// Remove removes the resource at the given path
func (c *Client) Remove(ctx context.Context, username, path string) error {
	log := appctx.GetLogger(ctx)

	nfo, err := c.GetFileInfoByPath(ctx, username, path)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("path", path).Str("err", err.Error())
		return err
	}

	if nfo.IsDir {
		return c.rmdir(ctx, username, path)
	}

	return c.rm(ctx, username, path)
}

// Rename renames the resource referenced by oldPath to newPath
func (c *Client) Rename(ctx context.Context, username, oldPath, newPath string) error {
	return errtypes.NotFound(fmt.Sprintf("%s:%s", "acltype", newPath))
}

// List the contents of the directory given by path
func (c *Client) List(ctx context.Context, username, dpath string) ([]*FileInfo, error) {
	log := appctx.GetLogger(ctx)

	// Stuff filename, uid, gid into the FindRequest type
	fdrq := new(erpc.FindRequest)
	fdrq.Maxdepth = 1
	fdrq.Type = erpc.TYPE_LISTING
	fdrq.Id = new(erpc.MDId)
	fdrq.Id.Path = []byte(dpath)

	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return nil, err
	}
	fdrq.Role = new(erpc.RoleId)

	uid, err := strconv.ParseUint(unixUser.Uid, 10, 64)
	if err != nil {
		return nil, err
	}
	fdrq.Role.Uid = uid
	gid, err := strconv.ParseUint(unixUser.Gid, 10, 64)
	if err != nil {
		return nil, err
	}
	fdrq.Role.Gid = gid

	fdrq.Authkey = c.opt.Authkey

	// Now send the req and see what happens
	resp, err := c.cl.Find(context.Background(), fdrq)
	if err != nil {
		log.Error().Err(err).Str("username", username).Str("path", dpath).Str("err", err.Error())

		return nil, err
	}

	var mylst []*FileInfo
	i := 0
	for {
		rsp, err := resp.Recv()
		if err != nil {
			if err == io.EOF {
				return mylst, nil
			}

			log.Warn().Err(err).Str("username", username).Str("path", dpath).Str("err", err.Error())

			return nil, err
		}

		if rsp == nil {
			log.Warn().Err(err).Str("username", username).Str("path", dpath).Str("err", "rsp is nil")
			return nil, errtypes.NotFound(dpath)
		}

		log.Debug().Str("username", username).Str("path", dpath).Str("item resp:", fmt.Sprintf("%#v", rsp)).Msg("grpc response")

		myitem, err := c.grpcMDResponseToFileInfo(rsp, dpath)
		if err != nil {
			log.Warn().Err(err).Str("username", username).Str("path", dpath).Str("could not convert item:", fmt.Sprintf("%#v", rsp)).Str("err:", err.Error()).Msg("")

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
func (c *Client) Read(ctx context.Context, username, path string) (io.ReadCloser, error) {
	unixUser, err := c.getUnixUser(username)
	if err != nil {
		return nil, err
	}
	uuid := uuid.Must(uuid.NewV4())
	rand := "eosread-" + uuid.String()
	localTarget := fmt.Sprintf("%s/%s", c.opt.CacheDirectory, rand)
	xrdPath := fmt.Sprintf("%s//%s", c.opt.URL, path)
	cmd := exec.CommandContext(ctx, c.opt.XrdcopyBinary, "--nopbar", "--silent", "-f", xrdPath, localTarget, fmt.Sprintf("-OSeos.ruid=%s&eos.rgid=%s", unixUser.Uid, unixUser.Gid))
	_, _, err = c.execute(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return os.Open(localTarget)
}

// Write writes a file to the mgm
func (c *Client) Write(ctx context.Context, username, path string, stream io.ReadCloser) error {
	unixUser, err := c.getUnixUser(username)
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
	cmd := exec.CommandContext(ctx, c.opt.XrdcopyBinary, "--nopbar", "--silent", "-f", fd.Name(), xrdPath, fmt.Sprintf("-ODeos.ruid=%s&eos.rgid=%s", unixUser.Uid, unixUser.Gid))
	_, _, err = c.execute(ctx, cmd)
	return err
}

// ListDeletedEntries returns a list of the deleted entries.
func (c *Client) ListDeletedEntries(ctx context.Context, username string) ([]*DeletedEntry, error) {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return nil, err
	}

	msg := new(erpc.NSRequest_RecycleRequest)
	msg.Cmd = erpc.NSRequest_RecycleRequest_RECYCLE_CMD(erpc.NSRequest_RecycleRequest_RECYCLE_CMD_value["LIST"])

	rq.Command = &erpc.NSRequest_Recycle{Recycle: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(context.Background(), rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("err", err.Error())
		return nil, err
	}

	if resp == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil response for username: '%s'", username))
	}

	log.Info().Str("username", username).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	// TODO(labkode): add protection if slave is configured and alive to count how many files are in the trashbin before
	// triggering the recycle ls call that could break the instance because of unavailable memory.
	// FF: I agree with labkode, if we think we may have memory problems then the semantics of the grpc call`and
	// the semantics if this func will have to change. For now this is not foreseen

	ret := make([]*DeletedEntry, 0)
	for _, f := range resp.Recycle.Recycles {
		if f == nil {
			log.Info().Str("username", username).Msg("nil item in response")
			continue
		}

		entry := &DeletedEntry{
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
func (c *Client) RestoreDeletedEntry(ctx context.Context, username, key string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
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
		log.Warn().Err(err).Str("username", username).Str("key", key).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' key: '%s'", username, key))
	}

	log.Info().Str("username", username).Str("key", key).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	return err
}

// PurgeDeletedEntries purges all entries from the recycle bin.
func (c *Client) PurgeDeletedEntries(ctx context.Context, username string) error {
	log := appctx.GetLogger(ctx)

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(username)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_RecycleRequest)
	msg.Cmd = erpc.NSRequest_RecycleRequest_RECYCLE_CMD(erpc.NSRequest_RecycleRequest_RECYCLE_CMD_value["PURGE"])

	rq.Command = &erpc.NSRequest_Recycle{Recycle: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(context.Background(), rq)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Str("err", err.Error())
		return err
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for username: '%s' ", username))
	}

	log.Info().Str("username", username).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	return err
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
	// TODO(ffurano):
	/*
		unixUser, err := c.getUnixUser(username)
		if err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, c.opt.EosBinary, "-r", unixUser.Uid, unixUser.Gid, "file", "versions", path, version)
		_, _, err = c.executeEOS(ctx, cmd)
		return err
	*/
	return errtypes.NotSupported("TODO")
}

// ReadVersion reads the version for the given file.
func (c *Client) ReadVersion(ctx context.Context, username, p, version string) (io.ReadCloser, error) {
	basename := path.Base(p)
	versionFile := path.Join(path.Dir(p), versionPrefix+basename, version)
	return c.Read(ctx, username, versionFile)
}

func (c *Client) grpcMDResponseToFileInfo(st *erpc.MDResponse, namepfx string) (*FileInfo, error) {
	if st.Cmd == nil && st.Fmd == nil {
		return nil, errors.Wrap(errtypes.NotSupported(""), "Invalid response (st.Cmd and st.Fmd are nil)")
	}
	fi := new(FileInfo)

	if st.Type != 0 {
		fi.IsDir = true
	}
	if st.Fmd != nil {
		fi.Inode = st.Fmd.Id
		fi.UID = st.Fmd.Uid
		fi.GID = st.Fmd.Gid
		fi.MTimeSec = st.Fmd.Mtime.Sec
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
		if namepfx == "" {
			fi.File = string(st.Cmd.Name)
		} else {
			fi.File = namepfx + "/" + string(st.Cmd.Name)
		}

		for k, v := range st.Cmd.Xattrs {
			if fi.Attrs == nil {
				fi.Attrs = make(map[string]string)
			}
			fi.Attrs[k] = string(v)
		}

		fi.Size = 0
	}

	fi.ETag = fi.Attrs["etag"]

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
