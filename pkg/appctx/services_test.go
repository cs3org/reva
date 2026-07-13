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

import "testing"

func TestGRPCServiceForMethod(t *testing.T) {
	MapGRPCService("cs3.gateway.v1beta1.GatewayAPI", "gateway")

	if name, ok := GRPCServiceForMethod("/cs3.gateway.v1beta1.GatewayAPI/Authenticate"); !ok || name != "gateway" {
		t.Fatalf("want gateway, got %q ok=%v", name, ok)
	}
	if _, ok := GRPCServiceForMethod("/cs3.unknown.v1beta1.NopeAPI/Method"); ok {
		t.Fatal("unknown proto service must not resolve")
	}
}
