// Copyright 2018-2026 CERN
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


package admin

import (
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"google.golang.org/grpc"
)

// The typed services are separate types, so that a method name of one service
// cannot collide with a method of another.
type spacesAdmin struct {
	adminpb.UnimplementedSpacesAdminServer
	s *svc
}

type sharesAdmin struct {
	adminpb.UnimplementedSharesAdminServer
	s *svc
}

type storageAdmin struct {
	adminpb.UnimplementedStorageAdminServer
	s *svc
}

// register registers every Admin API service on one server. The gRPC listener
// and the local socket must serve the same services.
func (s *svc) register(ss grpc.ServiceRegistrar) {
	adminpb.RegisterAdminAPIServer(ss, s)
	adminpb.RegisterSpacesAdminServer(ss, &spacesAdmin{s: s})
	adminpb.RegisterSharesAdminServer(ss, &sharesAdmin{s: s})
	adminpb.RegisterStorageAdminServer(ss, &storageAdmin{s: s})
}
