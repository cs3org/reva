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

package main

import (
	// These are all the extensions points for REVA
	_ "github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/interceptors/loader"
	_ "github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/loader"
	_ "github.com/cs3org/reva/cmd/revad/svcs/httpsvcs/handlers/auth/credential/loader"
	_ "github.com/cs3org/reva/cmd/revad/svcs/httpsvcs/handlers/auth/token/loader"
	_ "github.com/cs3org/reva/cmd/revad/svcs/httpsvcs/handlers/auth/tokenwriter/loader"
	_ "github.com/cs3org/reva/cmd/revad/svcs/httpsvcs/handlers/loader"
	_ "github.com/cs3org/reva/cmd/revad/svcs/httpsvcs/loader"
	_ "github.com/cs3org/reva/pkg/auth/manager/loader"
	_ "github.com/cs3org/reva/pkg/storage/broker/loader"
	_ "github.com/cs3org/reva/pkg/storage/fs/loader"
	_ "github.com/cs3org/reva/pkg/token/manager/loader"
	_ "github.com/cs3org/reva/pkg/user/manager/loader"
)
