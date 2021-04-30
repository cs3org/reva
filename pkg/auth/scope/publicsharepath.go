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

package scope

import (
	"fmt"
	"strings"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/utils"
)

func publicsharepathScope(scope *authpb.Scope, resource interface{}) (bool, error) {
	var ref provider.Reference
	err := utils.UnmarshalJSONToProtoV1(scope.Resource.Value, &ref)
	if err != nil {
		return false, err
	}

	switch v := resource.(type) {
	// Viewer role
	case *registry.GetStorageProvidersRequest:
		return strings.HasPrefix(v.GetRef().GetPath(), ref.GetPath()), nil
	case *provider.StatRequest:
		return strings.HasPrefix(v.GetRef().GetPath(), ref.GetPath()), nil
	case *provider.ListContainerRequest:
		return strings.HasPrefix(v.GetRef().GetPath(), ref.GetPath()), nil
	case *provider.InitiateFileDownloadRequest:
		return strings.HasPrefix(v.GetRef().GetPath(), ref.GetPath()), nil

		// Editor role
		// TODO(ishank011): Add role checks,
		// need to return appropriate status codes in the ocs/ocdav layers.
	case *provider.CreateContainerRequest:
		return strings.HasPrefix(v.GetRef().GetPath(), ref.GetPath()), nil
	case *provider.DeleteRequest:
		return strings.HasPrefix(v.GetRef().GetPath(), ref.GetPath()), nil
	case *provider.MoveRequest:
		return strings.HasPrefix(v.GetSource().GetPath(), ref.GetPath()) && strings.HasPrefix(v.GetDestination().GetPath(), ref.GetPath()), nil
	case *provider.InitiateFileUploadRequest:
		return strings.HasPrefix(v.GetRef().GetPath(), ref.GetPath()), nil
	}

	return false, errtypes.InternalError(fmt.Sprintf("resource type assertion failed: %+v", resource))
}
