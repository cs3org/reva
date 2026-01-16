package eosgrpc

import (
	"context"
	"fmt"
	"time"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/eosclient"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// ListDeletedEntries returns a list of the deleted entries.
func (c *Client) ListDeletedEntries(ctx context.Context, auth eosclient.Authorization, recycleid string, maxentries int, from, to time.Time) ([]*eosclient.DeletedEntry, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "ListDeletedEntries").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return nil, err
	}

	ret := make([]*eosclient.DeletedEntry, 0)
	count := 0
	
	recycleType := erpc.RecycleProto_UID
	if recycleid != "" {
		recycleType = erpc.RecycleProto_RID
	}
	
	for d := to; !d.Before(from); d = d.AddDate(0, 0, -1) {
		rq.Command = &erpc.NSRequest_Recycle{
			Recycle: &erpc.RecycleProto{
				Subcmd: &erpc.RecycleProto_Ls{
					Ls: &erpc.RecycleProto_LsProto{
						Type: recycleType,
						FullDetails: true,
						MonitorFmt: true,
						Maxentries:  int32(maxentries + 1),
						Date: fmt.Sprintf("%04d/%02d/%02d", d.Year(), d.Month(), d.Day()),
						RecycleId: recycleid,
					},
				},
			},
		}

		// Now send the req and see what happens
		resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
		e := c.getRespError(resp, err)
		if e != nil {
			log.Error().Str("err", e.Error()).Msg("")
			return nil, e
		}

		if resp == nil {
			return nil, errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s'", auth.Role.UID))
		}

		if resp.GetError() != nil {
			log.Error().Str("func", "ListDeletedEntries").Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("EOS negative resp")
		} else {
			count += len(resp.Recycle.Recycles)
			log.Debug().Str("func", "ListDeletedEntries").Int("totalcount", count).Msg("grpc response")
		}

		if count > maxentries {
			return nil, errtypes.BadRequest("list too long")
		}

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
	}

	return ret, nil
}

// RestoreDeletedEntry restores a deleted entry.
func (c *Client) RestoreDeletedEntry(ctx context.Context, auth eosclient.Authorization, key string) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "RestoreDeletedEntries").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("key", key).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return err
	}
	
	rq.Command = &erpc.NSRequest_Recycle{
		Recycle: &erpc.RecycleProto{
			Subcmd: &erpc.RecycleProto_Restore{
				Restore: &erpc.RecycleProto_RestoreProto{
					Key: key,
				},
			},
		},
	}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "RestoreDeletedEntries").Str("key", key).Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' key: '%s'", auth.Role.UID, key))
	}

	if resp.GetError() != nil {
		log.Error().Str("func", "RestoreDeletedEntries").Str("key", key).Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("EOS negative resp")
	} else {
		log.Info().Str("func", "RestoreDeletedEntries").Str("key", key).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")
	}
	return err
}

// PurgeDeletedEntries purges all entries from the recycle bin.
func (c *Client) PurgeDeletedEntries(ctx context.Context, auth eosclient.Authorization) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "PurgeDeletedEntries").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return err
	}
	
	rq.Command = &erpc.NSRequest_Recycle{
		Recycle: &erpc.RecycleProto{
			Subcmd: &erpc.RecycleProto_Purge{
				Purge: &erpc.RecycleProto_PurgeProto{},
			},
		},
	}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "PurgeDeletedEntries").Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' ", auth.Role.UID))
	}

	log.Info().Str("func", "PurgeDeletedEntries").Int64("errcode", resp.GetError().Code).Str("errmsg", resp.GetError().Msg).Msg("grpc response")

	return err
}
