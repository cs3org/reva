package eosgrpc

import (
	"context"
	"fmt"
	"path"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
	"github.com/pkg/errors"
)

// AddACL adds an new acl to EOS with the given aclType.
func (c *Client) AddACL(ctx context.Context, auth eosclient.Authorization, path string, pos uint, a *acl.Entry, recursive bool) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "AddACL").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Str("acl", a.CitrineSerialize()).Msg("")

	// Workaround: sudo'ers can set system attributes, but they cannot list directories
	// which means that they cannot set attributes recursively.
	// To fix this, we request the gid of `daemon`, which can read,
	// while keeping the uid of the sudo'er (cbox)
	auth.Role.GID = "2"

	// Init a new NSRequest
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return err
	}

	rq.Role.Gid = 2

	msg := new(erpc.NSRequest_AclRequest)
	msg.Cmd = erpc.NSRequest_AclRequest_ACL_COMMAND(erpc.NSRequest_AclRequest_ACL_COMMAND_value["MODIFY"])
	msg.Type = erpc.NSRequest_AclRequest_ACL_TYPE(erpc.NSRequest_AclRequest_ACL_TYPE_value["SYS_ACL"])
	msg.Recursive = recursive
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

	if resp.Acl != nil && resp.Acl.Code != 0 {
		return fmt.Errorf("Got error from EOS: code %d with message %s", resp.Acl.Code, resp.Acl.Msg)
	}

	log.Debug().Str("func", "AddACL").Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Any("acl", resp.Acl).Any("error", resp.Error).Msg("grpc response")

	return err
}

// RemoveACL removes the acl from EOS.
func (c *Client) RemoveACL(ctx context.Context, auth eosclient.Authorization, path string, a *acl.Entry, recursive bool) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "RemoveACL").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Str("ACL", a.CitrineSerialize()).Msg("")

	// We set permissions to "", so the ACL will serialize to `u:123456=`, which will make EOS delete the entry
	a.Permissions = ""
	err := c.AddACL(ctx, auth, path, eosclient.StartPosition, a, recursive)
	// If there are no ACLs left after the remove, EOS returns ENODATA. But that is not an actual error for us.
	if errors.Is(err, eosclient.NoDataError) {
		return nil
	}
	return err
}

// UpdateACL updates the EOS acl.
func (c *Client) UpdateACL(ctx context.Context, auth eosclient.Authorization, path string, position uint, a *acl.Entry, recursive bool) error {
	return c.AddACL(ctx, auth, path, position, a, recursive)
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

func (c *Client) fixupACLsAndAttrs(ctx context.Context, auth eosclient.Authorization, info *eosclient.FileInfo) *eosclient.FileInfo {
	// info.SysACL already holds the file's own ACL, parsed from its sys.acl xattr in
	// grpcMDResponseToFileInfo. Ensure it is non-nil so we can merge into it.
	if info.SysACL == nil {
		info.SysACL = &acl.ACLs{}
	}

	// For files we combine three ACL sources by precedence (lowest to highest): the parent
	// directory's inherited ACLs, the file's own entries, and the version folder's entries
	// (the canonical store for grants, also mirrored onto the file). We also merge the
	// attributes persisted on the version folder.
	if !info.IsDir {
		// Inherited parent ACLs are the base; the file's own entries take precedence.
		parentInfo, err := c.GetFileInfoByPath(ctx, auth, path.Dir(info.File))
		// Even if this call fails, at least return the current file object
		if err == nil && parentInfo.SysACL != nil {
			info.SysACL.Entries = eosclient.MergeACLEntries(parentInfo.SysACL.Entries, info.SysACL.Entries)
		}

		versionFolderInfo, err := c.GetFileInfoByPath(ctx, auth, eosclient.GetVersionFolder(info.File))
		if err == nil {
			// The version folder is the canonical store for grants, so its entries win.
			if versionFolderInfo.SysACL != nil {
				info.SysACL.Entries = eosclient.MergeACLEntries(info.SysACL.Entries, versionFolderInfo.SysACL.Entries)
			}
			if info.Attrs == nil {
				info.Attrs = map[string]string{}
			}
			for k, v := range versionFolderInfo.Attrs {
				if k == "sys.acl" {
					// sys.acl is represented in SysACL, merged above; don't shadow it as a raw attr.
					continue
				}
				info.Attrs[k] = v
			}
		}
	}
	return info
}
