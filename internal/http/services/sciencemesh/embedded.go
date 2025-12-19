// Copyright 2018-2025 CERN
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

package sciencemesh

import (
	"context"
	"encoding/json"
	"net/http"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocgraph"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/trace"
	"google.golang.org/grpc/codes"
	rpcstatus "google.golang.org/grpc/status"

	ocmconversions "github.com/cs3org/reva/v3/pkg/ocm/conversions"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	libregraph "github.com/owncloud/libre-graph-api-go"
)

type embeddedHandler struct {
	gatewayClient gateway.GatewayAPIClient
	config        *config
}

func (h *embeddedHandler) init(c *config) error {
	var err error
	h.gatewayClient, err = pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	if err != nil {
		return err
	}
	h.config = c

	return nil
}

// ListEmbeddedShares lists all embedded ocm shares for the current user.
func (h *embeddedHandler) ListEmbeddedShares(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	ocm_shares, err := h.gatewayClient.ListReceivedOCMShares(ctx, &ocm.ListReceivedOCMSharesRequest{
		Filters: []*ocm.ListReceivedOCMSharesRequest_Filter{
			{
				Type: ocm.ListReceivedOCMSharesRequest_Filter_TYPE_SHARE_TYPE,
				Term: &ocm.ListReceivedOCMSharesRequest_Filter_SharedResourceType{
					SharedResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED,
				},
			},
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("error listing ocm embedded shares")
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error listing ocm embedded shares", err)
		return
	}
	// Create the OCM converter
	converter := ocmconversions.NewConverter(h.gatewayClient, &ocmconversions.Config{
		WebBase: "", // sciencemesh doesn't have a web base configured
	})

	// Create a wrapper function to convert UnifiedRoleDefinition
	roleConverter := func(ctx context.Context, perms *storageprovider.ResourcePermissions) *ocmconversions.UnifiedRoleDefinition {
		role := CS3ResourcePermissionsToUnifiedRole(ctx, perms)
		if role == nil {
			return nil
		}
		return &ocmconversions.UnifiedRoleDefinition{
			Id: role.Id,
		}
	}

	shares := make([]*libregraph.DriveItem, 0)
	for _, share := range ocm_shares.Shares {
		drive, err := converter.OCMReceivedShareToDriveItem(ctx, share, roleConverter)
		if err != nil {
			log.Error().Err(err).Any("share", share).Msg("error parsing received share, ignoring")
		} else {
			shares = append(shares, drive)
		}
		log.Debug().Any("share", share).Msg("processing received ocm share")
	}

	if err := json.NewEncoder(w).Encode(map[string]any{
		"value": shares,
		"state": CreateStateMapping(ctx, ocm_shares.Shares),
	}); err != nil {
		w.Header().Set("x-request-id", trace.Get(ctx))
		code := rpcstatus.Code(err)
		if code == codes.Internal {
			log.Error().Err(err).Msg("embedded error")
		} else {
			log.Info().Err(err).Msg("embedded error")
		}
		w.WriteHeader(grpcCodeToHTTPStatus(code))
		w.Write([]byte("Error: " + err.Error()))

	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

func CS3ResourcePermissionsToUnifiedRole(ctx context.Context, p *storageprovider.ResourcePermissions) *libregraph.UnifiedRoleDefinition {
	log := appctx.GetLogger(ctx)
	role := conversions.RoleFromResourcePermissions(p)
	log.Debug().Interface("role", role).Interface("perms", p).Msg("Converting cs3 resource permissions to unified role")
	return ocsRoleUnifiedRole[role.Name]
}

func CreateStateMapping(ctx context.Context, receivedOCMShare []*ocm.ReceivedShare) map[string]string {
	stateMapping := make(map[string]string)
	for _, share := range receivedOCMShare {
		stateMapping[share.Id.OpaqueId] = convertShareState(share.State)
	}
	return stateMapping
}

func convertShareState(state ocm.ShareState) string {
	stateMapping := map[ocm.ShareState]string{
		ocm.ShareState_SHARE_STATE_INVALID:  "invalid",
		ocm.ShareState_SHARE_STATE_PENDING:  "pending",
		ocm.ShareState_SHARE_STATE_ACCEPTED: "accepted",
		ocm.ShareState_SHARE_STATE_REJECTED: "rejected",
	}
	if val, ok := stateMapping[state]; ok {
		return val
	}
	return "unknown"
}

var ocsRoleUnifiedRole = map[string]*libregraph.UnifiedRoleDefinition{
	conversions.RoleViewer:     ocgraph.NewViewerUnifiedRole(),
	conversions.RoleReader:     ocgraph.NewViewerUnifiedRole(),
	conversions.RoleEditor:     ocgraph.NewEditorUnifiedRole(),
	conversions.RoleFileEditor: ocgraph.NewFileEditorUnifiedRole(),
	conversions.RoleUploader:   ocgraph.NewUploaderUnifiedRole(),
	conversions.RoleManager:    ocgraph.NewManagerUnifiedRole(),
	conversions.RoleDenied:     ocgraph.NewAccessDeniedUnifiedRole(),
}

func grpcCodeToHTTPStatus(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		return 499 // Client Closed Request (non-standard)
	case codes.Unknown:
		return http.StatusInternalServerError
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusBadRequest
	case codes.Aborted:
		return http.StatusConflict
	case codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Internal:
		return http.StatusInternalServerError
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DataLoss:
		return http.StatusInternalServerError
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}
