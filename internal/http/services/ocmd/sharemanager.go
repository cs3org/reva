package ocmd

import (
	"context"
	"fmt"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/rs/zerolog"
)

func (s *svc) GetInternalShare(ctx context.Context, id string) (*share, error) {
	panic("implement me")
}

func (s *svc) NewShare(ctx context.Context, share *share, domain, shareWith string) (*share, error) {
	panic("implement me")
}

func (s *svc) GetShares(logger *zerolog.Logger, ctx context.Context, user string) ([]*share, error) {

	gateway, err := pool.GetGatewayServiceClient(s.Conf.GatewaySvc)
	if err != nil {
		return nil, err
	}

	filters := []*link.ListPublicSharesRequest_Filter{}
	req := link.ListPublicSharesRequest{
		Filters: filters,
	}

	logger.Debug().Str("gateway", fmt.Sprintf("%+v", gateway)).Str("req", fmt.Sprintf("%+v", req)).Msg("GetShares")

	res, err := gateway.ListPublicShares(ctx, &req)

	logger.Debug().Str("response", fmt.Sprintf("%+v", res)).Str("err", fmt.Sprintf("%+v", err)).Msg("GetShares")

	if err != nil {
		return nil, err
	}

	shares := make([]*share, 0)

	for i, publicShare := range res.GetShare() {
		logger.Debug().Str("idx", string(i)).Str("share", fmt.Sprintf("%+v", publicShare)).Msg("GetShares")

		share := convertPublicShareToShare(publicShare)
		shares = append(shares, share)
	}

	logger.Debug().Str("shares", fmt.Sprintf("%+v", shares)).Msg("GetShares")
	return shares, nil
}

func (s *svc) GetExternalShare(ctx context.Context, sharedWith, id string) (*share, error) {
	panic("implement me")
}

func convertPublicShareToShare(publicShare *link.PublicShare) *share {

	return &share{
		ID: publicShare.GetId().String(),
	}
}
