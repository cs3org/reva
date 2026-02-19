package eosgrpc

import (
	"context"
	"fmt"
	"strconv"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/trace"
)

// GetQuota gets the quota of a user on the quota node defined by path.
func (c *Client) GetQuota(ctx context.Context, user eosclient.Authorization, rootAuth eosclient.Authorization, path string) (*eosclient.QuotaInfo, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "GetQuota").Str("rootuid,rootgid", rootAuth.Role.UID+","+rootAuth.Role.GID).Any("user", user).Str("path", path).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, rootAuth, "")
	if err != nil {
		return nil, err
	}

	msg := new(erpc.NSRequest_QuotaRequest)
	msg.Path = []byte(path)
	msg.Id = new(erpc.RoleId)
	msg.Op = erpc.QUOTAOP_GET
	// Eos filters the returned quotas by username. This means that EOS must know it, someone
	// must have created an user with that name

	uid, err := strconv.ParseUint(user.Role.UID, 10, 64)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(user.Role.GID, 10, 64)
	if err != nil {
		return nil, err
	}

	if gid == 99 {
		msg.Id.Gid = gid
	} else {
		msg.Id.Uid = uid
	}
	msg.Id.Trace = trace.Get(ctx)
	rq.Command = &erpc.NSRequest_Quota{Quota: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		return nil, e
	}

	if resp == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil response for quota to user: '%s' path: '%s'", user, path))
	}

	if resp.GetError() != nil {
		log.Error().Str("func", "GetQuota").Any("user", user).Str("info:", fmt.Sprintf("%#v", resp)).Int64("eoserrcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("EOS negative resp")
	} else {
		log.Debug().Str("func", "GetQuota").Any("user", user).Str("info:", fmt.Sprintf("%#v", resp)).Msg("grpc response")
	}

	if resp.Quota == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil quota response? path: '%s'", path))
	}

	if resp.Quota.Code != 0 {
		return nil, errtypes.InternalError(fmt.Sprintf("Quota error from eos. info: '%#v'", resp.Quota))
	}

	// Let's loop on all the quotas that match this uid (apparently there can be many)
	// If there are many for this node, we sum them up
	qi := new(eosclient.QuotaInfo)
	for i := 0; i < len(resp.Quota.Quotanode); i++ {
		log.Debug().Str("func", "GetQuota").Str("quotanode:", fmt.Sprintf("%d: %#v", i, resp.Quota.Quotanode[i])).Msg("")

		qi.TotalBytes += max(uint64(resp.Quota.Quotanode[i].Maxlogicalbytes), 0)
		qi.UsedBytes += resp.Quota.Quotanode[i].Usedlogicalbytes

		qi.TotalInodes += max(uint64(resp.Quota.Quotanode[i].Maxfiles), 0)
		qi.UsedInodes += resp.Quota.Quotanode[i].Usedfiles
	}

	return qi, err
}

// SetQuota sets the quota of a user on the quota node defined by path.
func (c *Client) SetQuota(ctx context.Context, user eosclient.Authorization, rootAuth eosclient.Authorization, info *eosclient.SetQuotaInfo) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "SetQuota").Str("info:", fmt.Sprintf("%#v", info)).Msg("")

	// EOS does not have yet this command... work in progress, this is a draft piece of code
	// return errtypes.NotSupported("eosgrpc: SetQuota not implemented")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, rootAuth, "")
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_QuotaRequest)
	msg.Path = []byte(info.QuotaNode)
	msg.Id = new(erpc.RoleId)
	uidInt, err := strconv.ParseUint(info.UID, 10, 64)
	if err != nil {
		return err
	}

	if user.Role.GID == eosclient.ProjectQuotaGID {
		// new style project quota: we set a group quota on the space itself
		msg.Id.Uid = 0
		projectGID, _ := strconv.ParseUint(eosclient.ProjectQuotaGID, 10, 64)
		msg.Id.Gid = projectGID
	} else {
		// We set a quota for a user, not a group!
		msg.Id.Uid = uidInt
		msg.Id.Gid = 0
	}
	msg.Id.Username = info.Username
	msg.Id.Trace = trace.Get(ctx)
	msg.Op = erpc.QUOTAOP_SET
	msg.Maxbytes = info.MaxBytes
	msg.Maxfiles = info.MaxFiles
	rq.Command = &erpc.NSRequest_Quota{Quota: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for info: '%#v'", info))
	}

	if resp.GetError() != nil {
		log.Error().Str("func", "SetQuota").Str("info:", fmt.Sprintf("%#v", resp)).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("EOS negative resp")
	} else {
		log.Debug().Str("func", "SetQuota").Str("info:", fmt.Sprintf("%#v", resp)).Msg("grpc response")
	}

	if resp.Quota == nil {
		return errtypes.InternalError(fmt.Sprintf("nil quota response? info: '%#v'", info))
	}

	if resp.Quota.Code != 0 {
		return errtypes.InternalError(fmt.Sprintf("Quota error from eos. quota: '%#v'", resp.Quota))
	}

	log.Debug().Str("func", "GetQuota").Str("quotanodes", fmt.Sprintf("%d", len(resp.Quota.Quotanode))).Msg("grpc response")

	return err
}
