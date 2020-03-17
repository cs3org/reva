package ocmd

import "context"

func (s *svc) GetInternalShare(ctx context.Context, id string) (*share, error) {
	panic("implement me")
}

func (s *svc) NewShare(ctx context.Context, share *share, domain, shareWith string) (*share, error) {
	panic("implement me")
}

func (s *svc) GetShares(ctx context.Context, user string) ([]*share, error) {
	panic("implement me")
}

func (s *svc) GetExternalShare(ctx context.Context, sharedWith, id string) (*share, error) {
	panic("implement me")
}
