// Copyright 2018-2024 CERN
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

package service

import (
	"context"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	applicationauth "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authprovider "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	authregistry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	group "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	labels "github.com/cs3org/go-cs3apis/cs3/labels/v1beta1"
	ocmincoming "github.com/cs3org/go-cs3apis/cs3/ocm/incoming/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	preferences "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storageregistry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
)

// Each getter bakes in its peer name and builds the CS3 client over the
// resolved connection.

func (c *clients) Gateway(ctx context.Context) (gateway.GatewayAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameGateway)
	if err != nil {
		return nil, err
	}
	return gateway.NewGatewayAPIClient(conn), nil
}

func (c *clients) StorageProvider(ctx context.Context) (storageprovider.ProviderAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameStorageProvider)
	if err != nil {
		return nil, err
	}
	return storageprovider.NewProviderAPIClient(conn), nil
}

func (c *clients) StorageRegistry(ctx context.Context) (storageregistry.RegistryAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameStorageRegistry)
	if err != nil {
		return nil, err
	}
	return storageregistry.NewRegistryAPIClient(conn), nil
}

func (c *clients) Spaces(ctx context.Context) (storageprovider.SpacesAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameSpaces)
	if err != nil {
		return nil, err
	}
	return storageprovider.NewSpacesAPIClient(conn), nil
}

func (c *clients) AuthProvider(ctx context.Context) (authprovider.ProviderAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameAuthProvider)
	if err != nil {
		return nil, err
	}
	return authprovider.NewProviderAPIClient(conn), nil
}

func (c *clients) AuthRegistry(ctx context.Context) (authregistry.RegistryAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameAuthRegistry)
	if err != nil {
		return nil, err
	}
	return authregistry.NewRegistryAPIClient(conn), nil
}

func (c *clients) AppAuthProvider(ctx context.Context) (applicationauth.ApplicationsAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameAppAuthProvider)
	if err != nil {
		return nil, err
	}
	return applicationauth.NewApplicationsAPIClient(conn), nil
}

func (c *clients) UserProvider(ctx context.Context) (user.UserAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameUserProvider)
	if err != nil {
		return nil, err
	}
	return user.NewUserAPIClient(conn), nil
}

func (c *clients) GroupProvider(ctx context.Context) (group.GroupAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameGroupProvider)
	if err != nil {
		return nil, err
	}
	return group.NewGroupAPIClient(conn), nil
}

func (c *clients) UserShareProvider(ctx context.Context) (collaboration.CollaborationAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameUserShare)
	if err != nil {
		return nil, err
	}
	return collaboration.NewCollaborationAPIClient(conn), nil
}

func (c *clients) PublicShareProvider(ctx context.Context) (link.LinkAPIClient, error) {
	conn, _, err := c.resolve(ctx, NamePublicShare)
	if err != nil {
		return nil, err
	}
	return link.NewLinkAPIClient(conn), nil
}

func (c *clients) OCMShareProvider(ctx context.Context) (ocm.OcmAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameOCMShare)
	if err != nil {
		return nil, err
	}
	return ocm.NewOcmAPIClient(conn), nil
}

func (c *clients) OCMInviteManager(ctx context.Context) (invitepb.InviteAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameOCMInvite)
	if err != nil {
		return nil, err
	}
	return invitepb.NewInviteAPIClient(conn), nil
}

func (c *clients) OCMProviderAuthorizer(ctx context.Context) (ocmprovider.ProviderAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameOCMProvider)
	if err != nil {
		return nil, err
	}
	return ocmprovider.NewProviderAPIClient(conn), nil
}

func (c *clients) OCMIncoming(ctx context.Context) (ocmincoming.OcmIncomingAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameOCMIncoming)
	if err != nil {
		return nil, err
	}
	return ocmincoming.NewOcmIncomingAPIClient(conn), nil
}

func (c *clients) Preferences(ctx context.Context) (preferences.PreferencesAPIClient, error) {
	conn, _, err := c.resolve(ctx, NamePreferences)
	if err != nil {
		return nil, err
	}
	return preferences.NewPreferencesAPIClient(conn), nil
}

func (c *clients) Permissions(ctx context.Context) (permissions.PermissionsAPIClient, error) {
	conn, _, err := c.resolve(ctx, NamePermissions)
	if err != nil {
		return nil, err
	}
	return permissions.NewPermissionsAPIClient(conn), nil
}

func (c *clients) AppRegistry(ctx context.Context) (appregistry.RegistryAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameAppRegistry)
	if err != nil {
		return nil, err
	}
	return appregistry.NewRegistryAPIClient(conn), nil
}

func (c *clients) AppProvider(ctx context.Context) (appprovider.ProviderAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameAppProvider)
	if err != nil {
		return nil, err
	}
	return appprovider.NewProviderAPIClient(conn), nil
}

func (c *clients) DataTx(ctx context.Context) (datatx.TxAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameDataTx)
	if err != nil {
		return nil, err
	}
	return datatx.NewTxAPIClient(conn), nil
}

func (c *clients) Labels(ctx context.Context) (labels.LabelsAPIClient, error) {
	conn, _, err := c.resolve(ctx, NameLabels)
	if err != nil {
		return nil, err
	}
	return labels.NewLabelsAPIClient(conn), nil
}
