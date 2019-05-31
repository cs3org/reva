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

package loader

import (
	// Load core gRPC services.
	_ "github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/appprovidersvc"
	_ "github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/appregistrysvc"
	_ "github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/authsvc"
	_ "github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/shareregistrysvc"
	_ "github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/storageprovidersvc"
	_ "github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/storageregistrysvc"
	// Add your own service here
)
