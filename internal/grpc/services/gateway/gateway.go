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

package gateway

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/token/manager/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("gateway", New)
}

type config struct {
	AuthRegistryEndpoint          string `mapstructure:"authregistrysvc"`
	ApplicationAuthEndpoint       string `mapstructure:"applicationauthsvc"`
	StorageRegistryEndpoint       string `mapstructure:"storageregistrysvc"`
	AppRegistryEndpoint           string `mapstructure:"appregistrysvc"`
	PreferencesEndpoint           string `mapstructure:"preferencessvc"`
	UserShareProviderEndpoint     string `mapstructure:"usershareprovidersvc"`
	PublicShareProviderEndpoint   string `mapstructure:"publicshareprovidersvc"`
	OCMShareProviderEndpoint      string `mapstructure:"ocmshareprovidersvc"`
	OCMInviteManagerEndpoint      string `mapstructure:"ocminvitemanagersvc"`
	OCMProviderAuthorizerEndpoint string `mapstructure:"ocmproviderauthorizersvc"`
	OCMCoreEndpoint               string `mapstructure:"ocmcoresvc"`
	UserProviderEndpoint          string `mapstructure:"userprovidersvc"`
	GroupProviderEndpoint         string `mapstructure:"groupprovidersvc"`
	DataTxEndpoint                string `mapstructure:"datatx"`
	DataGatewayEndpoint           string `mapstructure:"datagateway"`
	PermissionsEndpoint           string `mapstructure:"permissionssvc"`
	CommitShareToStorageGrant     bool   `mapstructure:"commit_share_to_storage_grant"`
	CommitShareToStorageRef       bool   `mapstructure:"commit_share_to_storage_ref"`
	DisableHomeCreationOnLogin    bool   `mapstructure:"disable_home_creation_on_login"`
	TransferSharedSecret          string `mapstructure:"transfer_shared_secret"`
	TransferExpires               int64  `mapstructure:"transfer_expires"`
	TokenManager                  string `mapstructure:"token_manager"`
	// ShareFolder is the location where to create shares in the recipient's storage provider.
	ShareFolder         string                            `mapstructure:"share_folder"`
	DataTransfersFolder string                            `mapstructure:"data_transfers_folder"`
	HomeMapping         string                            `mapstructure:"home_mapping"`
	TokenManagers       map[string]map[string]interface{} `mapstructure:"token_managers"`
	EtagCacheTTL        int                               `mapstructure:"etag_cache_ttl"`
	AllowedUserAgents   map[string][]string               `mapstructure:"allowed_user_agents"` // map[path][]user-agent
	CreateHomeCacheTTL  int                               `mapstructure:"create_home_cache_ttl"`
	HomeLayout          string                            `mapstructure:"home_layout"`
}

// sets defaults.
func (c *config) ApplyDefaults() {
	if c.ShareFolder == "" {
		c.ShareFolder = "MyShares"
	}

	c.ShareFolder = strings.Trim(c.ShareFolder, "/")

	if c.TokenManager == "" {
		c.TokenManager = "jwt"
	}

	// if services address are not specified we used the shared conf
	// for the gatewaysvc to have dev setups very quickly.
	c.AuthRegistryEndpoint = sharedconf.GetGatewaySVC(c.AuthRegistryEndpoint)
	c.ApplicationAuthEndpoint = sharedconf.GetGatewaySVC(c.ApplicationAuthEndpoint)
	c.StorageRegistryEndpoint = sharedconf.GetGatewaySVC(c.StorageRegistryEndpoint)
	c.AppRegistryEndpoint = sharedconf.GetGatewaySVC(c.AppRegistryEndpoint)
	c.PreferencesEndpoint = sharedconf.GetGatewaySVC(c.PreferencesEndpoint)
	c.UserShareProviderEndpoint = sharedconf.GetGatewaySVC(c.UserShareProviderEndpoint)
	c.PublicShareProviderEndpoint = sharedconf.GetGatewaySVC(c.PublicShareProviderEndpoint)
	c.OCMShareProviderEndpoint = sharedconf.GetGatewaySVC(c.OCMShareProviderEndpoint)
	c.OCMInviteManagerEndpoint = sharedconf.GetGatewaySVC(c.OCMInviteManagerEndpoint)
	c.OCMProviderAuthorizerEndpoint = sharedconf.GetGatewaySVC(c.OCMProviderAuthorizerEndpoint)
	c.OCMCoreEndpoint = sharedconf.GetGatewaySVC(c.OCMCoreEndpoint)
	c.UserProviderEndpoint = sharedconf.GetGatewaySVC(c.UserProviderEndpoint)
	c.GroupProviderEndpoint = sharedconf.GetGatewaySVC(c.GroupProviderEndpoint)
	c.DataTxEndpoint = sharedconf.GetGatewaySVC(c.DataTxEndpoint)

	c.DataGatewayEndpoint = sharedconf.GetDataGateway(c.DataGatewayEndpoint)

	// use shared secret if not set
	c.TransferSharedSecret = sharedconf.GetJWTSecret(c.TransferSharedSecret)

	// lifetime for the transfer token (TUS upload)
	if c.TransferExpires == 0 {
		c.TransferExpires = 100 * 60 // seconds
	}

	// default to /home
	if c.HomeLayout == "" {
		c.HomeLayout = "/home"
	}
}

type svc struct {
	c               *config
	dataGatewayURL  url.URL
	tokenmgr        token.Manager
	etagCache       *ttlcache.Cache `mapstructure:"etag_cache"`
	createHomeCache *ttlcache.Cache `mapstructure:"create_home_cache"`
}

// New creates a new gateway svc that acts as a proxy for any grpc operation.
// The gateway is responsible for high-level controls: rate-limiting, coordination between svcs
// like sharing and storage acls, asynchronous transactions, ...
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	// ensure DataGatewayEndpoint is a valid URI
	u, err := url.Parse(c.DataGatewayEndpoint)
	if err != nil {
		return nil, err
	}

	tokenManager, err := getTokenManager(c.TokenManager, c.TokenManagers)
	if err != nil {
		return nil, err
	}

	etagCache := ttlcache.NewCache()
	_ = etagCache.SetTTL(time.Duration(c.EtagCacheTTL) * time.Second)
	etagCache.SkipTTLExtensionOnHit(true)

	createHomeCache := ttlcache.NewCache()
	_ = createHomeCache.SetTTL(time.Duration(c.CreateHomeCacheTTL) * time.Second)
	createHomeCache.SkipTTLExtensionOnHit(true)

	s := &svc{
		c:               &c,
		dataGatewayURL:  *u,
		tokenmgr:        tokenManager,
		etagCache:       etagCache,
		createHomeCache: createHomeCache,
	}

	return s, nil
}

func (s *svc) Register(ss *grpc.Server) {
	gateway.RegisterGatewayAPIServer(ss, s)
}

func (s *svc) Close() error {
	s.etagCache.Close()
	return nil
}

func (s *svc) UnprotectedEndpoints() []string {
	return []string{
		"/cs3.gateway.v1beta1.GatewayAPI/ListShare",
		"/cs3.gateway.v1beta1.GatewayAPI/GetAppPassword",
		"/cs3.gateway.v1beta1.GatewayAPI/AddAppProvider",
		"/cs3.gateway.v1beta1.GatewayAPI/ListSupportedMimeTypes",
		"/cs3.gateway.v1beta1.GatewayAPI/Authenticate",
		"/cs3.gateway.v1beta1.GatewayAPI/GetAuthProvider",
		"/cs3.gateway.v1beta1.GatewayAPI/ListAuthProviders",
		"/cs3.gateway.v1beta1.GatewayAPI/CreateOCMCoreShare",
		"/cs3.gateway.v1beta1.GatewayAPI/AcceptInvite",
		"/cs3.gateway.v1beta1.GatewayAPI/GetAcceptedUser",
		"/cs3.gateway.v1beta1.GatewayAPI/IsProviderAllowed",
		"/cs3.gateway.v1beta1.GatewayAPI/ListAllProviders",
		"/cs3.gateway.v1beta1.GatewayAPI/GetOCMShareByToken",
		"/cs3.gateway.v1beta1.GatewayAPI/GetPublicShareByToken",
		"/cs3.gateway.v1beta1.GatewayAPI/GetUser",
		"/cs3.gateway.v1beta1.GatewayAPI/GetUserByClaim",
		"/cs3.gateway.v1beta1.GatewayAPI/GetUserGroups",

		"/cs3.auth.applications.v1beta1.ApplicationsAPI/GetAppPassword",
		"/cs3.app.registry.v1beta1.RegistryAPI/AddAppProvider",
		"/cs3.app.registry.v1beta1.RegistryAPI/ListSupportedMimeTypes",
		"/cs3.auth.provider.v1beta1.ProviderAPI/Authenticate",
		"/cs3.auth.registry.v1beta1.RegistryAPI/GetAuthProvider",
		"/cs3.auth.registry.v1beta1.RegistryAPI/ListAuthProviders",
		"/cs3.ocm.core.v1beta1.OcmCoreAPI/CreateOCMCoreShare",
		"/cs3.ocm.invite.v1beta1.InviteAPI/AcceptInvite",
		"/cs3.ocm.invite.v1beta1.InviteAPI/GetAcceptedUser",
		"/cs3.ocm.provider.v1beta1.ProviderAPI/IsProviderAllowed",
		"/cs3.ocm.provider.v1beta1.ProviderAPI/ListAllProviders",
		"/cs3.sharing.ocm.v1beta1.OcmAPI/GetOCMShareByToken",
		"/cs3.sharing.link.v1beta1.LinkAPI/GetPublicShareByToken",
		"/cs3.identity.user.v1beta1.UserAPI/GetUser",
		"/cs3.identity.user.v1beta1.UserAPI/GetUserByClaim",
		"/cs3.identity.user.v1beta1.UserAPI/GetUserGroups",
	}
}

func getTokenManager(manager string, m map[string]map[string]interface{}) (token.Manager, error) {
	if f, ok := registry.NewFuncs[manager]; ok {
		return f(m[manager])
	}

	return nil, errtypes.NotFound(fmt.Sprintf("driver %s not found for token manager", manager))
}
