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

package loader

import (
	// Load core HTTP services.
	_ "github.com/cs3org/reva/v3/internal/http/services/appprovider"
	_ "github.com/cs3org/reva/v3/internal/http/services/archiver"
	_ "github.com/cs3org/reva/v3/internal/http/services/datagateway"
	_ "github.com/cs3org/reva/v3/internal/http/services/dataprovider"
	_ "github.com/cs3org/reva/v3/internal/http/services/experimental/overleaf"
	_ "github.com/cs3org/reva/v3/internal/http/services/helloworld"
	_ "github.com/cs3org/reva/v3/internal/http/services/metrics"
	_ "github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	_ "github.com/cs3org/reva/v3/internal/http/services/owncloud/ocapi"
	_ "github.com/cs3org/reva/v3/internal/http/services/owncloud/ocdav"
	_ "github.com/cs3org/reva/v3/internal/http/services/owncloud/ocgraph"
	_ "github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs"
	_ "github.com/cs3org/reva/v3/internal/http/services/pingpong"
	_ "github.com/cs3org/reva/v3/internal/http/services/plugins"
	_ "github.com/cs3org/reva/v3/internal/http/services/pprof"
	_ "github.com/cs3org/reva/v3/internal/http/services/preferences"
	_ "github.com/cs3org/reva/v3/internal/http/services/prometheus"
	_ "github.com/cs3org/reva/v3/internal/http/services/sciencemesh"
	_ "github.com/cs3org/reva/v3/internal/http/services/wellknown"
	// Add your own service here.
)
