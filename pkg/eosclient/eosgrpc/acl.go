package eosgrpc

import (
	"context"
	"fmt"
	"path"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/eosclient"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
)

// AddACL adds an new acl to EOS with the given aclType.
func (c *Client) AddACL(ctx context.Context, auth, rootAuth eosclient.Authorization, path string, pos uint, a *acl.Entry) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "AddACL").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Str("acl", a.CitrineSerialize()).Msg("")

	// First, we need to figure out if the path is a directory
	// to know whether our request should be recursive
	fileInfo, err := c.GetFileInfoByPath(ctx, auth, path)
	if err != nil {
		return err
	}

	// Init a new NSRequest
	rq, err := c.initNSRequest(ctx, rootAuth, "")
	if err != nil {
		return err
	}

	// Workaround: sudo'ers can set system attributes, but they cannot list directories
	// which means that they cannot set attributes recursively.
	// To fix this, we request the gid of `daemon`, which can read,
	// while keeping the uid of the sudo'er (cbox)
	rq.Role.Gid = 2

	msg := new(erpc.NSRequest_AclRequest)
	msg.Cmd = erpc.NSRequest_AclRequest_ACL_COMMAND(erpc.NSRequest_AclRequest_ACL_COMMAND_value["MODIFY"])
	msg.Type = erpc.NSRequest_AclRequest_ACL_TYPE(erpc.NSRequest_AclRequest_ACL_TYPE_value["SYS_ACL"])
	msg.Recursive = fileInfo.IsDir
	msg.Rule = a.CitrineSerialize()

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Acl{Acl: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "AddACL").Str("path", path).Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.NotFound(fmt.Sprintf("Path: %s", path))
	}

	log.Debug().Str("func", "AddACL").Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err
}

// RemoveACL removes the acl from EOS.
func (c *Client) RemoveACL(ctx context.Context, auth, rootAuth eosclient.Authorization, path string, a *acl.Entry) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "RemoveACL").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Str("ACL", a.CitrineSerialize()).Msg("")

	// We set permissions to "", so the ACL will serialize to `u:123456=`, which will make EOS delete the entry
	a.Permissions = ""
	return c.AddACL(ctx, auth, rootAuth, path, eosclient.StartPosition, a)
}

// UpdateACL updates the EOS acl.
func (c *Client) UpdateACL(ctx context.Context, auth, rootAuth eosclient.Authorization, path string, position uint, a *acl.Entry) error {
	return c.AddACL(ctx, auth, rootAuth, path, position, a)
}

// GetACL for a file.
func (c *Client) GetACL(ctx context.Context, auth eosclient.Authorization, path, aclType, target string) (*acl.Entry, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "GetACL").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

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
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "ListACLs").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	parsedACLs, err := c.getACLForPath(ctx, auth, path)
	if err != nil {
		return nil, err
	}

	// EOS Citrine ACLs are stored with uid. The UID will be resolved to the
	// user opaque ID at the eosfs level.
	return parsedACLs.Entries, nil
}

func (c *Client) getACLForPath(ctx context.Context, auth eosclient.Authorization, path string) (*acl.ACLs, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "GetACLForPath").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	fileInfo, err := c.GetFileInfoByPath(ctx, auth, path)
	if err != nil {
		return nil, err
	}

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return nil, err
	}

	msg := new(erpc.NSRequest_AclRequest)
	msg.Cmd = erpc.NSRequest_AclRequest_ACL_COMMAND(erpc.NSRequest_AclRequest_ACL_COMMAND_value["LIST"])
	msg.Type = erpc.NSRequest_AclRequest_ACL_TYPE(erpc.NSRequest_AclRequest_ACL_TYPE_value["SYS_ACL"])
	msg.Recursive = fileInfo.IsDir

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Acl{Acl: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "GetACLForPath").Str("path", path).Str("err", e.Error()).Msg("")
		return nil, e
	}

	if resp == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' path: '%s'", auth.Role.UID, path))
	}

	log.Debug().Str("func", "GetACLForPath").Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	if resp.Acl == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil acl for uid: '%s' path: '%s'", auth.Role.UID, path))
	}

	if resp.GetError() != nil {
		log.Error().Str("func", "GetACLForPath").Str("uid", auth.Role.UID).Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("EOS negative resp")
	}

	aclret, err := acl.Parse(resp.Acl.Rule, acl.ShortTextForm)

	// Now loop and build the correct return value

	return aclret, err
}

func (c *Client) fixupACLs(ctx context.Context, auth eosclient.Authorization, info *eosclient.FileInfo) *eosclient.FileInfo {
	// Append the ACLs that are described by the xattr sys.acl entry
	a, err := acl.Parse(info.Attrs["sys.acl"], acl.ShortTextForm)
	if err == nil {
		if info.SysACL != nil {
			info.SysACL.Entries = append(info.SysACL.Entries, a.Entries...)
		} else {
			info.SysACL = a
		}
	}

	// We need to inherit the ACLs for the parent directory as these are not available for files
	if !info.IsDir {
		parentInfo, err := c.GetFileInfoByPath(ctx, auth, path.Dir(info.File))
		// Even if this call fails, at least return the current file object
		if err == nil {
			info.SysACL.Entries = append(info.SysACL.Entries, parentInfo.SysACL.Entries...)
		}
	}
	return info
}
