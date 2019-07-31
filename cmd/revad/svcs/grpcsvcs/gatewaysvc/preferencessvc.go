package gatewaysvc

import (
	"context"

	preferencesv0alphapb "github.com/cs3org/go-cs3apis/cs3/preferences/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/pkg/errors"
)

func (s *svc) SetKey(ctx context.Context, req *preferencesv0alphapb.SetKeyRequest) (*preferencesv0alphapb.SetKeyResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetPreferencesClient(s.c.PreferencesEndpoint)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting preferences client")
		return &preferencesv0alphapb.SetKeyResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.SetKey(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling SetKey")
	}

	return res, nil
}

func (s *svc) GetKey(ctx context.Context, req *preferencesv0alphapb.GetKeyRequest) (*preferencesv0alphapb.GetKeyResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetPreferencesClient(s.c.PreferencesEndpoint)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting preferences client")
		return &preferencesv0alphapb.GetKeyResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.GetKey(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling GetKey")
	}

	return res, nil
}
