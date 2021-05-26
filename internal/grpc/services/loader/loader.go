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

package loader

import (
	// Load core gRPC services.
	_ "github.com/cs3org/reva/internal/grpc/services/applicationauth"
	_ "github.com/cs3org/reva/internal/grpc/services/appprovider"
	_ "github.com/cs3org/reva/internal/grpc/services/appregistry"
	_ "github.com/cs3org/reva/internal/grpc/services/authprovider"
	_ "github.com/cs3org/reva/internal/grpc/services/authregistry"
	_ "github.com/cs3org/reva/internal/grpc/services/datatx"
	_ "github.com/cs3org/reva/internal/grpc/services/gateway"
	_ "github.com/cs3org/reva/internal/grpc/services/groupprovider"
	_ "github.com/cs3org/reva/internal/grpc/services/helloworld"
	_ "github.com/cs3org/reva/internal/grpc/services/ocmcore"
	_ "github.com/cs3org/reva/internal/grpc/services/ocminvitemanager"
	_ "github.com/cs3org/reva/internal/grpc/services/ocmproviderauthorizer"
	_ "github.com/cs3org/reva/internal/grpc/services/ocmshareprovider"
	_ "github.com/cs3org/reva/internal/grpc/services/preferences"
	_ "github.com/cs3org/reva/internal/grpc/services/publicshareprovider"
	_ "github.com/cs3org/reva/internal/grpc/services/publicstorageprovider"
	_ "github.com/cs3org/reva/internal/grpc/services/storageprovider"
	_ "github.com/cs3org/reva/internal/grpc/services/storageregistry"
	_ "github.com/cs3org/reva/internal/grpc/services/userprovider"
	_ "github.com/cs3org/reva/internal/grpc/services/usershareprovider"
	// Add your own service here
)
