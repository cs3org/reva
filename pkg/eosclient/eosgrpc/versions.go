package eosgrpc

import (
	"context"
	"fmt"
	"io"
	"path"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/eosclient"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// ListVersions list all the versions for a given file.
func (c *Client) ListVersions(ctx context.Context, auth eosclient.Authorization, p string) ([]*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "ListVersions").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("p", p).Msg("")

	versionFolder := eosclient.GetVersionFolder(p)
	finfos, err := c.List(ctx, auth, versionFolder)
	if err != nil {
		return []*eosclient.FileInfo{}, err
	}
	return finfos, nil
}

// RollbackToVersion rollbacks a file to a previous version.
func (c *Client) RollbackToVersion(ctx context.Context, auth eosclient.Authorization, path, version string) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "RollbackToVersion").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Str("version", version).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_VersionRequest)
	msg.Cmd = erpc.NSRequest_VersionRequest_VERSION_CMD(erpc.NSRequest_VersionRequest_VERSION_CMD_value["GRAB"])
	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)
	msg.Grabversion = version

	rq.Command = &erpc.NSRequest_Version{Version: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "RollbackToVersion").Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' ", auth.Role.UID))
	}

	if resp.GetError() != nil {
		log.Info().Str("func", "RollbackToVersion").Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")
	}
	return nil
}

// ReadVersion reads the version for the given file.
func (c *Client) ReadVersion(ctx context.Context, auth eosclient.Authorization, p, version string) (io.ReadCloser, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "ReadVersion").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("p", p).Str("version", version).Msg("")

	versionFile := path.Join(eosclient.GetVersionFolder(p), version)
	return c.Read(ctx, auth, versionFile, nil)
}
