// Copyright 2018-2019 CERN
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

package gatewaysvc

import (
	"io"

	appproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/appprovider/v0alpha"
	appregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/appregistry/v0alpha"
	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	preferencesv0alphapb "github.com/cs3org/go-cs3apis/cs3/preferences/v0alpha"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storageregv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v0alpha"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"

	"github.com/cs3org/reva/cmd/revad/grpcserver"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	grpcserver.Register("gatewaysvc", New)
}

// New creates a new gateway svc that acts as a proxy for any grpc operation.
// The gateway is responsible for high-level controls: rate-limiting, coordination between svcs
// like sharing and storage acls, asynchronous transactions, ...
func New(m map[string]interface{}, ss *grpc.Server) (io.Closer, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	s := &svc{
		c: c,
	}

	storageproviderv0alphapb.RegisterStorageProviderServiceServer(ss, s)
	authv0alphapb.RegisterAuthServiceServer(ss, s)
	usershareproviderv0alphapb.RegisterUserShareProviderServiceServer(ss, s)
	appregistryv0alphapb.RegisterAppRegistryServiceServer(ss, s)
	appproviderv0alphapb.RegisterAppProviderServiceServer(ss, s)
	preferencesv0alphapb.RegisterPreferencesServiceServer(ss, s)
	storageregv0alphapb.RegisterStorageRegistryServiceServer(ss, s)

	return s, nil
}

type svc struct {
	c *config
}

func (s *svc) Close() error {
	return nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "gatewaysvc: error decoding conf")
		return nil, err
	}
	return c, nil
}

type config struct {
	StorageRegistryEndpoint   string `mapstructure:"storageregistrysvc"`
	AuthEndpoint              string `mapstructure:"authsvc"`
	AppRegistryEndpoint       string `mapstructure:"appregistrysvc"`
	PreferencesEndpoint       string `mapstructure:"preferencessvc"`
	UserShareProviderEndpoint string `mapstructure:"usershareprovidersvc"`
}
