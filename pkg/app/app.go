// Copyright 2018-2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package app

import (
	"context"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// Registry is the interface that application registries implement
// for discovering application providers
type Registry interface {
	FindProviders(ctx context.Context, mimeType string) ([]*registry.ProviderInfo, error)
	ListProviders(ctx context.Context) ([]*registry.ProviderInfo, error)
	AddProvider(ctx context.Context, p *registry.ProviderInfo) error
	GetDefaultProviderForMimeType(ctx context.Context, mimeType string) (*registry.ProviderInfo, error)
	SetDefaultProviderForMimeType(ctx context.Context, mimeType string, p *registry.ProviderInfo) error
}

// Provider is the interface that application providers implement
// for providing the URL of the app which will serve the requested resource.
type Provider interface {
	GetAppURL(ctx context.Context, resource *provider.ResourceInfo, viewMode appprovider.OpenInAppRequest_ViewMode, app, token string) (string, error)
}
