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

func (c *clients) Gateway(_ context.Context) (gateway.GatewayAPIClient, error) {
	conn, _, err := c.resolve(NameGateway)
	if err != nil {
		return nil, err
	}
	return gateway.NewGatewayAPIClient(conn), nil
}

func (c *clients) StorageProvider(_ context.Context) (storageprovider.ProviderAPIClient, error) {
	conn, _, err := c.resolve(NameStorageProvider)
	if err != nil {
		return nil, err
	}
	return storageprovider.NewProviderAPIClient(conn), nil
}

func (c *clients) StorageRegistry(_ context.Context) (storageregistry.RegistryAPIClient, error) {
	conn, _, err := c.resolve(NameStorageRegistry)
	if err != nil {
		return nil, err
	}
	return storageregistry.NewRegistryAPIClient(conn), nil
}

func (c *clients) Spaces(_ context.Context) (storageprovider.SpacesAPIClient, error) {
	conn, _, err := c.resolve(NameSpaces)
	if err != nil {
		return nil, err
	}
	return storageprovider.NewSpacesAPIClient(conn), nil
}

func (c *clients) AuthProvider(_ context.Context) (authprovider.ProviderAPIClient, error) {
	conn, _, err := c.resolve(NameAuthProvider)
	if err != nil {
		return nil, err
	}
	return authprovider.NewProviderAPIClient(conn), nil
}

func (c *clients) AuthRegistry(_ context.Context) (authregistry.RegistryAPIClient, error) {
	conn, _, err := c.resolve(NameAuthRegistry)
	if err != nil {
		return nil, err
	}
	return authregistry.NewRegistryAPIClient(conn), nil
}

func (c *clients) AppAuthProvider(_ context.Context) (applicationauth.ApplicationsAPIClient, error) {
	conn, _, err := c.resolve(NameAppAuthProvider)
	if err != nil {
		return nil, err
	}
	return applicationauth.NewApplicationsAPIClient(conn), nil
}

func (c *clients) UserProvider(_ context.Context) (user.UserAPIClient, error) {
	conn, _, err := c.resolve(NameUserProvider)
	if err != nil {
		return nil, err
	}
	return user.NewUserAPIClient(conn), nil
}

func (c *clients) GroupProvider(_ context.Context) (group.GroupAPIClient, error) {
	conn, _, err := c.resolve(NameGroupProvider)
	if err != nil {
		return nil, err
	}
	return group.NewGroupAPIClient(conn), nil
}

func (c *clients) UserShareProvider(_ context.Context) (collaboration.CollaborationAPIClient, error) {
	conn, _, err := c.resolve(NameUserShare)
	if err != nil {
		return nil, err
	}
	return collaboration.NewCollaborationAPIClient(conn), nil
}

func (c *clients) PublicShareProvider(_ context.Context) (link.LinkAPIClient, error) {
	conn, _, err := c.resolve(NamePublicShare)
	if err != nil {
		return nil, err
	}
	return link.NewLinkAPIClient(conn), nil
}

func (c *clients) OCMShareProvider(_ context.Context) (ocm.OcmAPIClient, error) {
	conn, _, err := c.resolve(NameOCMShare)
	if err != nil {
		return nil, err
	}
	return ocm.NewOcmAPIClient(conn), nil
}

func (c *clients) OCMInviteManager(_ context.Context) (invitepb.InviteAPIClient, error) {
	conn, _, err := c.resolve(NameOCMInvite)
	if err != nil {
		return nil, err
	}
	return invitepb.NewInviteAPIClient(conn), nil
}

func (c *clients) OCMProviderAuthorizer(_ context.Context) (ocmprovider.ProviderAPIClient, error) {
	conn, _, err := c.resolve(NameOCMProvider)
	if err != nil {
		return nil, err
	}
	return ocmprovider.NewProviderAPIClient(conn), nil
}

func (c *clients) OCMIncoming(_ context.Context) (ocmincoming.OcmIncomingAPIClient, error) {
	conn, _, err := c.resolve(NameOCMIncoming)
	if err != nil {
		return nil, err
	}
	return ocmincoming.NewOcmIncomingAPIClient(conn), nil
}

func (c *clients) Preferences(_ context.Context) (preferences.PreferencesAPIClient, error) {
	conn, _, err := c.resolve(NamePreferences)
	if err != nil {
		return nil, err
	}
	return preferences.NewPreferencesAPIClient(conn), nil
}

func (c *clients) Permissions(_ context.Context) (permissions.PermissionsAPIClient, error) {
	conn, _, err := c.resolve(NamePermissions)
	if err != nil {
		return nil, err
	}
	return permissions.NewPermissionsAPIClient(conn), nil
}

func (c *clients) AppRegistry(_ context.Context) (appregistry.RegistryAPIClient, error) {
	conn, _, err := c.resolve(NameAppRegistry)
	if err != nil {
		return nil, err
	}
	return appregistry.NewRegistryAPIClient(conn), nil
}

func (c *clients) AppProvider(_ context.Context) (appprovider.ProviderAPIClient, error) {
	conn, _, err := c.resolve(NameAppProvider)
	if err != nil {
		return nil, err
	}
	return appprovider.NewProviderAPIClient(conn), nil
}

func (c *clients) DataTx(_ context.Context) (datatx.TxAPIClient, error) {
	conn, _, err := c.resolve(NameDataTx)
	if err != nil {
		return nil, err
	}
	return datatx.NewTxAPIClient(conn), nil
}

func (c *clients) Labels(_ context.Context) (labels.LabelsAPIClient, error) {
	conn, _, err := c.resolve(NameLabels)
	if err != nil {
		return nil, err
	}
	return labels.NewLabelsAPIClient(conn), nil
}
