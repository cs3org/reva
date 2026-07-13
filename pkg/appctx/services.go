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

package appctx

import (
	"strings"
	"sync"
)

// grpcServices maps a gRPC proto service ("cs3.gateway.v1beta1.GatewayAPI") to
// the reva service that registered it ("gateway"), so per-request loggers can
// be stamped with the owning service.
var (
	grpcSvcMu    sync.RWMutex
	grpcServices = map[string]string{}
)

// MapGRPCService records that a proto service is implemented by a reva service.
func MapGRPCService(protoService, revaService string) {
	grpcSvcMu.Lock()
	grpcServices[protoService] = revaService
	grpcSvcMu.Unlock()
}

// GRPCServiceForMethod resolves a full gRPC method ("/pkg.Service/Method") to
// the reva service that registered it, if known.
func GRPCServiceForMethod(fullMethod string) (string, bool) {
	proto := strings.TrimPrefix(fullMethod, "/")
	if i := strings.Index(proto, "/"); i >= 0 {
		proto = proto[:i]
	}
	grpcSvcMu.RLock()
	defer grpcSvcMu.RUnlock()
	name, ok := grpcServices[proto]
	return name, ok
}
