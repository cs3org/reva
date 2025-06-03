package eosgrpc

import (
	"context"
	"fmt"
	"strings"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/eosclient"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/acl"
	"github.com/cs3org/reva/pkg/trace"
	"github.com/pkg/errors"
)

// SetAttr sets an extended attributes on a path.
func (c *Client) SetAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, errorIfExists, recursive bool, path, app string) error {
	log := appctx.GetLogger(ctx)

	if !isValidAttribute(attr) {
		return errors.New("eos: attr is invalid: " + serializeAttribute(attr))
	}

	log.Debug().Bool("recursive", recursive).Str("path", path).Any("attr", attr).Str("trace", trace.Get(ctx)).Msg("eos-grpc SetAttr()")
	// Favorites need to be stored per user so handle these separately
	if attr.Type == eosclient.UserAttr && attr.Key == favoritesKey {
		info, err := c.GetFileInfoByPath(ctx, auth, path)
		if err != nil {
			return err
		}
		return c.handleFavAttr(ctx, auth, attr, recursive, path, info, true)
	}
	return c.setEOSAttr(ctx, auth, attr, errorIfExists, recursive, path, app)
}

func (c *Client) setEOSAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, errorIfExists, recursive bool, path, app string) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "SetAttr").Bool("recursive", recursive).Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, app)
	if err != nil {
		return err
	}

	// Workaround: sudo'ers can set system attributes, but they cannot list directories
	// which means that they cannot set attributes recursively.
	// To fix this, we request the gid of `daemon`, which can read,
	// while keeping the uid of the sudo'er (cbox)
	rq.Role.Gid = 2

	msg := new(erpc.NSRequest_SetXAttrRequest)

	var m = map[string][]byte{attr.GetKey(): []byte(attr.Val)}
	msg.Xattrs = m
	msg.Recursive = recursive

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	if errorIfExists {
		msg.Create = true
	}

	rq.Command = &erpc.NSRequest_Xattr{Xattr: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)

	if resp != nil && resp.Error != nil && resp.Error.Code == 17 {
		return eosclient.AttrAlreadyExistsError
	}

	if e != nil {
		log.Error().Str("func", "SetAttr").Str("path", path).Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' gid: '%s' path: '%s'", auth.Role.UID, auth.Role.GID, path))
	}

	if resp.GetError() != nil {
		log.Error().Str("func", "setAttr").Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("EOS negative result")
	}

	return err
}

func (c *Client) handleFavAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, recursive bool, path string, info *eosclient.FileInfo, set bool) error {
	var err error
	u := appctx.ContextMustGetUser(ctx)
	if info == nil {
		info, err = c.GetFileInfoByPath(ctx, auth, path)
		if err != nil {
			return err
		}
	}
	favStr := info.Attrs[favoritesKey]
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

func (c *Client) unsetEOSAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, recursive bool, path, app string, deleteFavs bool) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "unsetEOSAttr").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	// Favorites need to be stored per user so handle these separately
	if !deleteFavs && attr.Type == eosclient.UserAttr && attr.Key == favoritesKey {
		info, err := c.GetFileInfoByPath(ctx, auth, path)
		if err != nil {
			return err
		}
		return c.handleFavAttr(ctx, auth, attr, recursive, path, info, false)
	}

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, app)
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_SetXAttrRequest)

	var ktd = []string{attr.GetKey()}
	msg.Keystodelete = ktd
	msg.Recursive = recursive
	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Xattr{Xattr: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)

	if resp != nil && resp.Error != nil && resp.Error.Code == 61 {
		return eosclient.AttrNotExistsError
	}

	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "UnsetAttr").Str("path", path).Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' gid: '%s' path: '%s'", auth.Role.UID, auth.Role.GID, path))
	}

	if resp.GetError() != nil {
		log.Error().Str("func", "UnsetAttr").Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("EOS negative resp")
	}
	return err
}

// GetAttr returns the attribute specified by key.
func (c *Client) GetAttr(ctx context.Context, auth eosclient.Authorization, key, path string) (*eosclient.Attribute, error) {
	info, err := c.GetFileInfoByPath(ctx, auth, path)
	if err != nil {
		return nil, err
	}

	for k, v := range info.Attrs {
		if k == key {
			attr, err := getAttribute(k, v)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("eosgrpc: cannot parse attribute key=%s value=%s", k, v))
			}
			return attr, nil
		}
	}
	return nil, errtypes.NotFound(fmt.Sprintf("key %s not found", key))
}

// GetAttrs returns all the attributes of a resource.
func (c *Client) GetAttrs(ctx context.Context, auth eosclient.Authorization, path string) ([]*eosclient.Attribute, error) {
	info, err := c.GetFileInfoByPath(ctx, auth, path)
	if err != nil {
		return nil, err
	}

	attrs := make([]*eosclient.Attribute, 0, len(info.Attrs))
	for k, v := range info.Attrs {
		attr, err := getAttribute(k, v)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("eosgrpc: cannot parse attribute key=%s value=%s", k, v))
		}
		attrs = append(attrs, attr)
	}

	return attrs, nil
}

func getAttribute(key, val string) (*eosclient.Attribute, error) {
	// key is in the form sys.forced.checksum
	type2key := strings.SplitN(key, ".", 2) // type2key = ["sys", "forced.checksum"]
	if len(type2key) != 2 {
		return nil, errtypes.InternalError("wrong attr format to deserialize")
	}
	t, err := eosclient.AttrStringToType(type2key[0])
	if err != nil {
		return nil, err
	}
	attr := &eosclient.Attribute{
		Type: t,
		Key:  type2key[1],
		Val:  val,
	}
	return attr, nil
}

func isValidAttribute(a *eosclient.Attribute) bool {
	// validate that an attribute is correct.
	if (a.Type != eosclient.SystemAttr && a.Type != eosclient.UserAttr) || a.Key == "" {
		return false
	}
	return true
}

func serializeAttribute(a *eosclient.Attribute) string {
	return fmt.Sprintf("%s.%s=%s", attrTypeToString(a.Type), a.Key, a.Val)
}
