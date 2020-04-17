// Copyright 2018-2020 CERN
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
	"fmt"
	"net/url"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"

	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/token/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("gateway", New)
}

type config struct {
	AuthRegistryEndpoint          string `mapstructure:"authregistrysvc"`
	StorageRegistryEndpoint       string `mapstructure:"storageregistrysvc"`
	AppRegistryEndpoint           string `mapstructure:"appregistrysvc"`
	PreferencesEndpoint           string `mapstructure:"preferencessvc"`
	UserShareProviderEndpoint     string `mapstructure:"usershareprovidersvc"`
	PublicShareProviderEndpoint   string `mapstructure:"publicshareprovidersvc"`
	OCMShareProviderEndpoint      string `mapstructure:"ocmshareprovidersvc"`
	OCMInviteManagerEndpoint      string `mapstructure:"ocminvitemanagersvc"`
	OCMProviderAuthorizerEndpoint string `mapstructure:"ocmproviderauthorizersvc"`
	UserProviderEndpoint          string `mapstructure:"userprovidersvc"`
	CommitShareToStorageGrant     bool   `mapstructure:"commit_share_to_storage_grant"`
	CommitShareToStorageRef       bool   `mapstructure:"commit_share_to_storage_ref"`
	DisableHomeCreationOnLogin    bool   `mapstructure:"disable_home_creation_on_login"`
	DataGatewayEndpoint           string `mapstructure:"datagateway"`
	TransferSharedSecret          string `mapstructure:"transfer_shared_secret"`
	TranserExpires                int64  `mapstructure:"transfer_expires"`
	TokenManager                  string `mapstructure:"token_manager"`
	// ShareFolder is the location where to create shares in the recipient's storage provider.
	ShareFolder   string                            `mapstructure:"share_folder"`
	TokenManagers map[string]map[string]interface{} `mapstructure:"token_managers"`
}

type svc struct {
	c              *config
	dataGatewayURL url.URL
	tokenmgr       token.Manager
}

// New creates a new gateway svc that acts as a proxy for any grpc operation.
// The gateway is responsible for high-level controls: rate-limiting, coordination between svcs
// like sharing and storage acls, asynchronous transactions, ...
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	// set defaults
	if c.ShareFolder == "" {
		c.ShareFolder = "MyShares"
	}

	c.ShareFolder = strings.Trim(c.ShareFolder, "/")

	if c.TokenManager == "" {
		c.TokenManager = "jwt"
	}

	// ensure DataGatewayEndpoint is a valid URI
	if c.DataGatewayEndpoint == "" {
		return nil, errors.New("datagateway is not defined")
	}

	u, err := url.Parse(c.DataGatewayEndpoint)
	if err != nil {
		return nil, err
	}

	tokenManager, err := getTokenManager(c.TokenManager, c.TokenManagers)
	if err != nil {
		return nil, err
	}

	s := &svc{
		c:              c,
		dataGatewayURL: *u,
		tokenmgr:       tokenManager,
	}

	return s, nil
}

func (s *svc) Register(ss *grpc.Server) {
	gateway.RegisterGatewayAPIServer(ss, s)
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) UnprotectedEndpoints() []string {
	return []string{"/cs3.gateway.v1beta1.GatewayAPI"}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "gateway: error decoding conf")
		return nil, err
	}
	return c, nil
}

func getTokenManager(manager string, m map[string]map[string]interface{}) (token.Manager, error) {
	if f, ok := registry.NewFuncs[manager]; ok {
		return f(m[manager])
	}

	return nil, fmt.Errorf("driver %s not found for token manager", manager)
}
