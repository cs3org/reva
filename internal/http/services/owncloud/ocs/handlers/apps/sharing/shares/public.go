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

package shares

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/v3/pkg/appctx"

	"github.com/cs3org/reva/v3/pkg/notification"
	"github.com/cs3org/reva/v3/pkg/publicshare"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

func (h *Handler) createPublicLinkShare(w http.ResponseWriter, r *http.Request, statInfo *provider.ResourceInfo) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	c, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	err = r.ParseForm()
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Could not parse form from request", err)
		return
	}

	if quicklink, _ := strconv.ParseBool(r.FormValue("quicklink")); quicklink {
		res, err := c.ListPublicShares(ctx, &link.ListPublicSharesRequest{
			Filters: []*link.ListPublicSharesRequest_Filter{
				publicshare.ResourceIDFilter(statInfo.Id),
			},
		})
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "could not list public links", err)
			return
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			response.WriteOCSError(w, r, int(res.Status.GetCode()), "could not list public links", nil)
			return
		}

		for _, l := range res.GetShare() {
			if l.Quicklink {
				s := conversions.PublicShare2ShareData(l, r, h.publicURL)
				err = h.addFileInfo(ctx, s, statInfo)
				if err != nil {
					response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error enhancing response with share data", err)
					return
				}
				h.mapUserIds(ctx, c, s)
				response.WriteOCSSuccess(w, r, s)
				return
			}
		}
	}

	newPermissions, err := permissionFromRequest(r, h)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Could not read permission from request", err)
		return
	}

	if newPermissions == nil {
		// default perms: read-only
		// TODO: the default might change depending on allowed permissions and configs
		newPermissions, err = ocPublicPermToCs3(1, h)
		if err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "Could not convert default permissions", err)
			return
		}
	}

	if statInfo != nil && statInfo.Type == provider.ResourceType_RESOURCE_TYPE_FILE {
		// Single file shares should never have delete or create permissions
		role := conversions.RoleFromResourcePermissions(newPermissions)
		permissions := role.OCSPermissions()
		permissions &^= conversions.PermissionCreate
		permissions &^= conversions.PermissionDelete
		newPermissions = conversions.RoleFromOCSPermissions(permissions).CS3ResourcePermissions()
	}

	internal, _ := strconv.ParseBool(r.FormValue("internal"))
	notifyUploads, _ := strconv.ParseBool(r.FormValue("notifyUploads"))
	notifyUploadsExtraRecipients := r.FormValue("notifyUploadsExtraRecipients")

	req := link.CreatePublicShareRequest{
		ResourceInfo: statInfo,
		Grant: &link.Grant{
			Permissions: &link.PublicSharePermissions{
				Permissions: newPermissions,
			},
			Password: r.FormValue("password"),
		},
		Description:                  r.FormValue("description"),
		Internal:                     internal,
		NotifyUploads:                notifyUploads,
		NotifyUploadsExtraRecipients: notifyUploadsExtraRecipients,
	}

	endOfDay := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 23, 59, 59, 0, time.Now().Location())
	maxExpiration := uint64(h.pubRWLinkMaxExpiration.Seconds())
	defaultExpiration := uint64(h.pubRWLinkDefaultExpiration.Seconds())
	totalMaxExpiration := uint64(endOfDay.Unix()) + maxExpiration
	totalDefaultExpiration := uint64(endOfDay.Unix()) + defaultExpiration

	expireTimeString, ok := r.Form["expireDate"]
	if ok {
		if expireTimeString[0] != "" {
			expireTime, err := conversions.ParseTimestamp(expireTimeString[0])
			if err != nil {
				response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "invalid datetime format", err)
				return
			}
			if isPermissionEditor(newPermissions) && expireTime.Seconds > totalMaxExpiration {
				response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "expiration date exceeds maximum allowed", err)
				return
			}
			if expireTime != nil {
				req.Grant.Expiration = expireTime
			}
		}
	} else if isPermissionEditor(newPermissions) && defaultExpiration != 0 {
		expireTime := &types.Timestamp{
			Seconds: totalDefaultExpiration,
		}
		req.Grant.Expiration = expireTime
	}

	// set displayname and password protected as arbitrary metadata
	req.ResourceInfo.ArbitraryMetadata = &provider.ArbitraryMetadata{
		Metadata: map[string]string{
			"name":      r.FormValue("name"),
			"quicklink": r.FormValue("quicklink"),
			// "password": r.FormValue("password"),
		},
	}

	createRes, err := c.CreatePublicShare(ctx, &req)
	if err != nil {
		log.Debug().Err(err).Str("createShare", "shares").Msgf("error creating a public share to resource id: %v", statInfo.GetId())
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error creating public share", fmt.Errorf("error creating a public share to resource id: %v", statInfo.GetId()))
		return
	}

	if createRes.Status.Code != rpc.Code_CODE_OK {
		log.Debug().Err(errors.New("create public share failed")).Str("shares", "createShare").Msgf("create public share failed with status code: %v", createRes.Status.Code.String())
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc create public share request failed", err)
		return
	}

	s := conversions.PublicShare2ShareData(createRes.Share, r, h.publicURL)
	err = h.addFileInfo(ctx, s, statInfo)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error enhancing response with share data", err)
		return
	}
	h.mapUserIds(ctx, c, s)

	response.WriteOCSSuccess(w, r, s)
}

func (h *Handler) listPublicShares(r *http.Request, filters []*link.ListPublicSharesRequest_Filter) ([]*conversions.ShareData, *rpc.Status, error) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	ocsDataPayload := make([]*conversions.ShareData, 0)
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		return ocsDataPayload, nil, err
	}

	req := link.ListPublicSharesRequest{
		Filters: filters,
	}

	res, err := client.ListPublicShares(ctx, &req)
	if err != nil {
		return ocsDataPayload, nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return ocsDataPayload, res.Status, nil
	}

	var wg sync.WaitGroup
	workers := 50
	input := make(chan *link.PublicShare, len(res.Share))
	output := make(chan *conversions.ShareData, len(res.Share))

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(ctx context.Context, client gateway.GatewayAPIClient, input chan *link.PublicShare, output chan *conversions.ShareData, wg *sync.WaitGroup) {
			defer wg.Done()

			for share := range input {
				info, status, err := h.getResourceInfoByID(ctx, client, share.ResourceId)
				if err != nil || status.Code != rpc.Code_CODE_OK {
					log.Debug().Interface("share", share.Id).Interface("status", status).Err(err).Msg("could not stat share, skipping")
					return
				}

				sData := conversions.PublicShare2ShareData(share, r, h.publicURL)

				sData.Name = share.DisplayName

				if err := h.addFileInfo(ctx, sData, info); err != nil {
					log.Debug().Interface("share", share.Id).Err(err).Msg("could not add file info, skipping")
					return
				}
				h.mapUserIds(ctx, client, sData)

				log.Debug().Interface("share", share.Id).Msg("mapped")
				output <- sData
			}
		}(ctx, client, input, output, &wg)
	}

	for _, share := range res.Share {
		input <- share
	}
	close(input)
	wg.Wait()
	close(output)

	for s := range output {
		ocsDataPayload = append(ocsDataPayload, s)
	}

	return ocsDataPayload, nil, nil
}

func (h *Handler) isPublicShare(r *http.Request, oid string) bool {
	logger := appctx.GetLogger(r.Context())
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		logger.Err(err).Send()
	}

	psRes, err := client.GetPublicShare(r.Context(), &link.GetPublicShareRequest{
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Id{
				Id: &link.PublicShareId{
					OpaqueId: oid,
				},
			},
		},
	})
	if err != nil {
		logger.Err(err).Send()
		return false
	}

	return psRes.GetShare() != nil
}

func (h *Handler) updatePublicShare(w http.ResponseWriter, r *http.Request, shareID string) {
	updates := []*link.UpdatePublicShareRequest_Update{}
	logger := appctx.GetLogger(r.Context())

	gwC, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		log.Err(err).Str("shareID", shareID).Msg("updatePublicShare")
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "error getting a connection to the gateway service", nil)
		return
	}

	before, err := gwC.GetPublicShare(r.Context(), &link.GetPublicShareRequest{
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Id{
				Id: &link.PublicShareId{
					OpaqueId: shareID,
				},
			},
		},
	})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "failed to get public share", nil)
		return
	}

	err = r.ParseForm()
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Could not parse form from request", err)
		return
	}

	// indicates whether values to update were found,
	// to check if the request was valid,
	// not whether an actual update has been performed
	updatesFound := false

	endOfDay := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 23, 59, 59, 0, time.Now().Location())
	maxExpiration := uint64(h.pubRWLinkMaxExpiration.Seconds())
	defaultExpiration := uint64(h.pubRWLinkDefaultExpiration.Seconds())
	totalMaxExpiration := uint64(endOfDay.Unix()) + maxExpiration
	totalDefaultExpiration := uint64(endOfDay.Unix()) + defaultExpiration

	newName, ok := r.Form["name"]
	if ok {
		updatesFound = true
		if newName[0] != before.Share.DisplayName {
			updates = append(updates, &link.UpdatePublicShareRequest_Update{
				Type:        link.UpdatePublicShareRequest_Update_TYPE_DISPLAYNAME,
				DisplayName: newName[0],
			})
		}
	}

	// Permissions
	newPermissions, err := permissionFromRequest(r, h)
	logger.Debug().Interface("newPermissions", newPermissions).Msg("Parsed permissions")
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "invalid permissions", err)
		return
	}

	// update permissions if given
	if newPermissions != nil {
		updatesFound = true
		publicSharePermissions := &link.PublicSharePermissions{
			Permissions: newPermissions,
		}
		beforePerm, _ := json.Marshal(before.GetShare().Permissions)
		afterPerm, _ := json.Marshal(publicSharePermissions)
		if string(beforePerm) != string(afterPerm) {
			logger.Info().Str("shares", "update").Msgf("updating permissions from %v to: %v", string(beforePerm), string(afterPerm))
			updates = append(updates, &link.UpdatePublicShareRequest_Update{
				Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
				Grant: &link.Grant{
					Permissions: publicSharePermissions,
				},
			})
		}

		// add default expiration date if it has 'editor' permissions
		if isPermissionEditor(newPermissions) && defaultExpiration != 0 {
			if before.Share.Expiration == nil {
				updates = append(updates, &link.UpdatePublicShareRequest_Update{
					Type: link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
					Grant: &link.Grant{
						Expiration: &types.Timestamp{
							Seconds: totalDefaultExpiration,
						},
					},
				})
			} else if maxExpiration != 0 {
				if before.Share.Expiration.Seconds > totalMaxExpiration {
					updates = append(updates, &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
						Grant: &link.Grant{
							Expiration: &types.Timestamp{
								Seconds: totalMaxExpiration,
							},
						},
					})
				}
			}
		}

		// remove notifications when a public link stops having 'uploader' permissions
		if !isPermissionUploader(newPermissions) {
			if before.Share.NotifyUploads {
				updates = append(updates, &link.UpdatePublicShareRequest_Update{
					Type:          link.UpdatePublicShareRequest_Update_TYPE_NOTIFYUPLOADS,
					NotifyUploads: false,
				})
			}

			if before.Share.NotifyUploadsExtraRecipients != "" {
				updates = append(updates, &link.UpdatePublicShareRequest_Update{
					Type:                         link.UpdatePublicShareRequest_Update_TYPE_NOTIFYUPLOADSEXTRARECIPIENTS,
					NotifyUploadsExtraRecipients: "",
				})
			}

			h.notificationHelper.UnregisterNotification(shareID)
		}
	}

	// ExpireDate
	expireTimeString, ok := r.Form["expireDate"]
	// check if value is set and must be updated or cleared
	if ok {
		updatesFound = true
		var newExpiration *types.Timestamp

		if expireTimeString[0] != "" {
			newExpiration, err = conversions.ParseTimestamp(expireTimeString[0])
			if err != nil {
				response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "invalid datetime format", err)
				return
			}
		}

		beforeExpiration, _ := json.Marshal(before.Share.Expiration)
		afterExpiration, _ := json.Marshal(newExpiration)
		if string(afterExpiration) != string(beforeExpiration) {
			if maxExpiration != 0 {
				if newPermissions != nil {
					if isPermissionEditor(newPermissions) && newExpiration.Seconds > totalMaxExpiration {
						response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "expiration date exceeds maximum allowed", err)
						return
					} else if isPermissionEditor(before.Share.Permissions.Permissions) && newExpiration.Seconds > totalMaxExpiration {
						response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "expiration date exceeds maximum allowed", err)
						return
					}
				}
			}

			logger.Debug().Str("shares", "update").Msgf("updating expire date from %v to: %v", string(beforeExpiration), string(afterExpiration))
			updates = append(updates, &link.UpdatePublicShareRequest_Update{
				Type: link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
				Grant: &link.Grant{
					Expiration: newExpiration,
				},
			})
		}
	}

	// Password
	newPassword, ok := r.Form["password"]
	// update or clear password
	if ok {
		updatesFound = true
		logger.Info().Str("shares", "update").Msg("password updated")
		updates = append(updates, &link.UpdatePublicShareRequest_Update{
			Type: link.UpdatePublicShareRequest_Update_TYPE_PASSWORD,
			Grant: &link.Grant{
				Password: newPassword[0],
			},
		})
	}

	// Description
	description, ok := r.Form["description"]
	if ok {
		updatesFound = true
		logger.Info().Str("shares", "update").Msg("description updated")
		updates = append(updates, &link.UpdatePublicShareRequest_Update{
			Type:        link.UpdatePublicShareRequest_Update_TYPE_DESCRIPTION,
			Description: description[0],
		})
	}

	// NotifyUploads
	newNotifyUploads, ok := r.Form["notifyUploads"]

	if ok {
		ok2 := permissionsStayUploader(before, newPermissions)
		u, ok3 := appctx.ContextGetUser(r.Context())

		if ok2 && ok3 {
			notifyUploads, _ := strconv.ParseBool(newNotifyUploads[0])
			updatesFound = true

			logger.Info().Str("shares", "update").Msgf("notify uploads updated to '%v'", notifyUploads)
			updates = append(updates, &link.UpdatePublicShareRequest_Update{
				Type:          link.UpdatePublicShareRequest_Update_TYPE_NOTIFYUPLOADS,
				NotifyUploads: notifyUploads,
			})

			if notifyUploads {
				n := &notification.Notification{
					TemplateName: "sharedfolder-upload-mail",
					Ref:          shareID,
					Recipients:   []string{u.Mail},
				}
				h.notificationHelper.RegisterNotification(n)
			} else {
				h.notificationHelper.UnregisterNotification(shareID)
			}
		}
	}

	// NotifyUploadsExtraRecipients
	newNotifyUploadsExtraRecipients, ok := r.Form["notifyUploadsExtraRecipients"]

	if ok {
		ok2 := permissionsStayUploader(before, newPermissions)
		u, ok3 := appctx.ContextGetUser(r.Context())

		if ok2 && ok3 {
			notifyUploadsExtraRecipients := newNotifyUploadsExtraRecipients[0]
			updatesFound = true
			logger.Info().Str("shares", "update").Msgf("notify uploads extra recipients updated to '%v'", notifyUploadsExtraRecipients)

			updates = append(updates, &link.UpdatePublicShareRequest_Update{
				Type:                         link.UpdatePublicShareRequest_Update_TYPE_NOTIFYUPLOADSEXTRARECIPIENTS,
				NotifyUploadsExtraRecipients: notifyUploadsExtraRecipients,
			})

			if len(notifyUploadsExtraRecipients) > 0 {
				n := &notification.Notification{
					TemplateName: "sharedfolder-upload-mail",
					Ref:          shareID,
					Recipients:   []string{u.Mail, notifyUploadsExtraRecipients},
				}
				h.notificationHelper.RegisterNotification(n)
			}
		}
	}

	publicShare := before.Share

	// The update API is atomic and requires a single property update at a time,
	// see: https://github.com/cs3org/cs3apis/pull/67#issuecomment-617651428
	if len(updates) > 0 {
		uRes := &link.UpdatePublicShareResponse{Share: before.Share}
		for k := range updates {
			uRes, err = gwC.UpdatePublicShare(r.Context(), &link.UpdatePublicShareRequest{
				Ref: &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: &link.PublicShareId{
							OpaqueId: shareID,
						},
					},
				},
				Update: updates[k],
			})
			if err != nil {
				log.Err(err).Str("shareID", shareID).Msg("sending update request to public link provider")
				response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "Error sending update request to public link provider", err)
				return
			}
		}
		publicShare = uRes.Share
	} else if !updatesFound {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "No updates specified in request", nil)
		return
	}

	statReq := provider.StatRequest{Ref: &provider.Reference{ResourceId: before.Share.ResourceId}}

	statRes, err := gwC.Stat(r.Context(), &statReq)
	if err != nil {
		log.Debug().Err(err).Str("shares", "update public share").Msg("error during stat")
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "missing resource information", fmt.Errorf("error getting resource information"))
		return
	}

	s := conversions.PublicShare2ShareData(publicShare, r, h.publicURL)
	err = h.addFileInfo(r.Context(), s, statRes.Info)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error enhancing response with share data", err)
		return
	}
	h.mapUserIds(r.Context(), gwC, s)

	response.WriteOCSSuccess(w, r, s)
}

func (h *Handler) removePublicShare(w http.ResponseWriter, r *http.Request, shareID string) {
	ctx := r.Context()

	c, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting grpc gateway client", err)
		return
	}

	req := &link.RemovePublicShareRequest{
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Id{
				Id: &link.PublicShareId{
					OpaqueId: shareID,
				},
			},
		},
	}

	res, err := c.RemovePublicShare(ctx, req)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error sending a grpc delete share request", err)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "not found", nil)
			return
		}
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "grpc delete share request failed", err)
		return
	}

	h.notificationHelper.UnregisterNotification(shareID)

	response.WriteOCSSuccess(w, r, nil)
}

func ocPublicPermToCs3(permKey int, h *Handler) (*provider.ResourcePermissions, error) {
	// TODO refactor this ocPublicPermToRole[permKey] check into a conversions.NewPublicSharePermissions?
	// not all permissions are possible for public shares
	_, ok := ocPublicPermToRole[permKey]
	if !ok {
		log.Error().Str("ocPublicPermToCs3", "shares").Int("perm", permKey).Msg("invalid public share permission")
		return nil, fmt.Errorf("invalid public share permission: %d", permKey)
	}

	perm, err := conversions.NewPermissions(permKey)
	if err != nil {
		return nil, err
	}

	return conversions.RoleFromOCSPermissions(perm).CS3ResourcePermissions(), nil
}

func permissionFromRequest(r *http.Request, h *Handler) (*provider.ResourcePermissions, error) {
	var err error
	// phoenix sends: {"permissions": 15}. See ocPublicPermToRole struct for mapping

	permKey := 1

	// note: "permissions" value has higher priority than "publicUpload"

	// handle legacy "publicUpload" arg that overrides permissions differently depending on the scenario
	// https://github.com/owncloud/core/blob/v10.4.0/apps/files_sharing/lib/Controller/Share20OcsController.php#L447
	publicUploadString, ok := r.Form["publicUpload"]
	if ok {
		publicUploadFlag, err := strconv.ParseBool(publicUploadString[0])
		if err != nil {
			log.Error().Err(err).Str("publicUpload", publicUploadString[0]).Msg("could not parse publicUpload argument")
			return nil, err
		}

		if publicUploadFlag {
			// all perms except reshare
			permKey = 15
		}
	} else {
		permissionsString, ok := r.Form["permissions"]
		if !ok {
			// no permission values given
			return nil, nil
		}

		permKey, err = strconv.Atoi(permissionsString[0])
		if err != nil {
			log.Error().Str("permissionFromRequest", "shares").Msgf("invalid type: %T", permKey)
			return nil, fmt.Errorf("invalid type: %T", permKey)
		}
	}

	p, err := ocPublicPermToCs3(permKey, h)
	if err != nil {
		return nil, err
	}
	return p, err
}

func isPermissionUploader(permissions *provider.ResourcePermissions) bool {
	if permissions == nil {
		return false
	}

	publicSharePermissions := &link.PublicSharePermissions{
		Permissions: permissions,
	}
	return conversions.RoleFromResourcePermissions(publicSharePermissions.Permissions).Name == conversions.RoleUploader
}

func isPermissionEditor(permissions *provider.ResourcePermissions) bool {
	if permissions == nil {
		return false
	}

	publicSharePermissions := &link.PublicSharePermissions{
		Permissions: permissions,
	}
	return conversions.RoleFromResourcePermissions(publicSharePermissions.Permissions).Name == conversions.RoleEditor
}

func permissionsStayUploader(before *link.GetPublicShareResponse, newPermissions *provider.ResourcePermissions) bool {
	return (newPermissions == nil && isPermissionUploader(before.Share.GetPermissions().Permissions)) || isPermissionUploader(newPermissions)
}

// TODO: add mapping for user share permissions to role

// Maps oc10 public link permissions to roles.
var ocPublicPermToRole = map[int]string{
	// Recipients can view and download contents.
	1: "viewer",
	// Recipients can view, download and edit single files.
	3: "file-editor",
	// Recipients can view, download, edit, delete and upload contents
	15: "editor",
	// Recipients can upload but existing contents are not revealed
	4: "uploader",
	// Recipients can view, download and upload contents
	5: "contributor",
}
