package ocmd

import "context"

func (s *svc) IsProviderAllowed(ctx context.Context, domain string) error {
	panic("implement me")
}

func (s *svc) GetProviderInfoByDomain(ctx context.Context, domain string) (*providerInfo, error) {
	panic("implement me")
}

func (s *svc) AddProvider(ctx context.Context, p *providerInfo) error {
	panic("implement me")
}
