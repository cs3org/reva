package eosgrpc

import (
	"context"
	"fmt"
	"strings"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/trace"
	"github.com/pkg/errors"
)

// SetAttr sets an extended attribute on the exact path given. Routing of file xattrs to
// the version folder is decided by the eosfs layer, not here.
func (c *Client) SetAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, errorIfExists, recursive bool, path, app string) error {
	log := appctx.GetLogger(ctx)

	if !isValidAttribute(attr) {
		return errors.New("eos: attr is invalid: " + serializeAttribute(attr))
	}

	log.Debug().Bool("recursive", recursive).Str("path", path).Any("attr", attr).Str("trace", trace.Get(ctx)).Msg("eos-grpc SetAttr()")
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
		if resp.GetError().Code == 0 {
			log.Info().Str("func", "setAttr").Str("path", path).Str("errmsg", resp.GetError().Msg).Msg("EOS operation succeeded")
		} else {
			log.Error().Str("func", "setAttr").Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("EOS non-zero result")
		}
	}

	return err
}

// UnsetAttr unsets an extended attribute on the exact path given. Routing of file xattrs
// to the version folder is decided by the eosfs layer, not here.
func (c *Client) UnsetAttr(ctx context.Context, auth eosclient.Authorization, attr *eosclient.Attribute, recursive bool, path, app string) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "UnsetAttr").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

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
		if resp.GetError().Code == 0 {
			log.Info().Str("func", "UnsetAttr").Str("path", path).Str("errmsg", resp.GetError().Msg).Msg("EOS operation succeeded")
		} else {
			log.Error().Str("func", "UnsetAttr").Str("path", path).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("EOS non-zero result")
		}
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
			return getAttribute(k, v), nil
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
		attrs = append(attrs, getAttribute(k, v))
	}

	return attrs, nil
}

// getAttribute splits a key of the attribute map a FileInfo carries into the
// type and the key an Attribute is made of.
//
// That map is not keyed the way EOS keys its xattrs: grpcMDResponseToFileInfo
// keeps the "sys." prefix but strips "user.", so a user attribute arrives here
// under its bare name, dots and all - "reva.lockpayload", say, which names no
// type and whose key is the whole thing. Only a key that starts with a type EOS
// knows is split; everything else is a user attribute. Reading "reva" as the
// type used to fail the lookup, and with it every call that reads the
// attributes of a resource that has ever been locked.
func getAttribute(key, val string) *eosclient.Attribute {
	if t, k, found := strings.Cut(key, "."); found {
		if at, err := eosclient.AttrStringToType(t); err == nil {
			return &eosclient.Attribute{Type: at, Key: k, Val: val}
		}
	}
	return &eosclient.Attribute{Type: eosclient.UserAttr, Key: key, Val: val}
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
