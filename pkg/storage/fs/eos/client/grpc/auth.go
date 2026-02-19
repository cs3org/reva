package eosgrpc

import (
	"context"
	"fmt"
	"time"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
)

// GenerateToken returns a token on behalf of the resource owner to be used by lightweight accounts.
func (c *Client) GenerateToken(ctx context.Context, auth eosclient.Authorization, path string, a *acl.Entry) (string, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "GenerateToken").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		log.Error().Str("func", "GenerateToken").Str("err", err.Error()).Msg("Error on initNSRequest")
		return "", err
	}

	msg := new(erpc.NSRequest_TokenRequest)
	msg.Token = &erpc.ShareToken{}
	msg.Token.Token = &erpc.ShareProto{}
	msg.Token.Token.Permission = a.Permissions
	msg.Token.Token.Expires = uint64(time.Now().Add(time.Duration(c.opt.TokenExpiry) * time.Second).Unix())
	msg.Token.Token.Allowtree = true
	msg.Token.Token.Path = path

	rq.Command = &erpc.NSRequest_Token{
		Token: msg,
	}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "GenerateToken").Str("err", e.Error()).Msg("")
		return "", e
	}

	if resp == nil {
		log.Error().Str("func", "GenerateToken").Msg("nil grpc response")
		return "", errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' ", auth.Role.UID))
	}

	// For some reason, the token is embedded in the error, with error code 0
	if resp.GetError() != nil {
		if resp.GetError().Code == 0 {
			return resp.GetError().Msg, nil
		}
	}
	log.Error().Str("func", "GenerateToken").Msg("GenerateToken over gRPC expected an error but did not receive one")
	return "", err
}
