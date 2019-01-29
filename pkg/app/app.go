package app

import "context"

// Registry is the interface that application registries implement
// for discovering application providers
type Registry interface {
	FindProvider(ctx context.Context, ext, mimetype string) (*ProviderInfo, error)
}

// ProviderInfo contains the information
// about a Application Provider
type ProviderInfo struct {
	Location string
}

// Provider is the interface that application providers implement
// for providing the iframe location to a iframe UI Provider
type Provider interface {
	GetIFrame(ctx context.Context, fn, mimetype, token string) (string, error)
}
