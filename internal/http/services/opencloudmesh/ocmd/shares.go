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

package ocmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/pkg/errors"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmincoming "github.com/cs3org/go-cs3apis/cs3/ocm/incoming/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/go-playground/validator/v10"
	"github.com/studio-b12/gowebdav"
	"google.golang.org/grpc/metadata"
)

var validate = validator.New()

type sharesHandler struct {
	gatewayClient              gateway.GatewayAPIClient
	exposeRecipientDisplayName bool
	machineSecret              string
	autoAcceptProviders        []*regexp.Regexp
}

func (h *sharesHandler) init(c *config) error {
	var err error
	h.gatewayClient, err = pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	if err != nil {
		return err
	}
	h.exposeRecipientDisplayName = c.ExposeRecipientDisplayName
	h.machineSecret = c.MachineSecret
	for _, p := range c.AutoAcceptProviders {
		re, err := regexp.Compile(p)
		if err != nil {
			return errors.Wrapf(err, "invalid auto_accept_providers regex %q", p)
		}
		h.autoAcceptProviders = append(h.autoAcceptProviders, re)
	}
	return nil
}

// matchesAutoAccept reports whether the given sender provider domain matches any
// of the configured auto-accept provider regexes.
func (h *sharesHandler) matchesAutoAccept(domain string) bool {
	for _, re := range h.autoAcceptProviders {
		if re.MatchString(domain) {
			return true
		}
	}
	return false
}

// isAcceptedUser reports whether remoteUser is already an accepted user of the
// recipient, i.e. they know each other through a prior invitation flow.
func (h *sharesHandler) isAcceptedUser(ctx context.Context, recipient *userpb.User, remoteUser *userpb.UserId) bool {
	log := appctx.GetLogger(ctx)
	filter, err := utils.MarshalProtoV1ToJSON(recipient.Id)
	if err != nil {
		log.Error().Err(err).Msg("auto-accept: error marshalling recipient id")
		return false
	}
	res, err := h.gatewayClient.GetAcceptedUser(ctx, &invitepb.GetAcceptedUserRequest{
		RemoteUserId: remoteUser,
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"user-filter": {Decoder: "json", Value: filter},
			},
		},
	})
	return err == nil && res.Status.Code == rpc.Code_CODE_OK
}

// CreateShare implements the OCM /shares call and stores an incoming share
func (h *sharesHandler) CreateShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	req, err := getCreateShareRequest(r)
	// Log whitelist metadata only; incoming OCM share requests carry shared secrets in protocol options.
	logEvent := log.Info().Str("remote", r.RemoteAddr).Err(err)
	if req != nil {
		logEvent = logEvent.Str("sender", req.Sender).Str("resource_type", req.ResourceType)
	}
	logEvent.Msg("OCM /shares request received")
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, err.Error(), nil)
		return
	}

	sender, err := GetUserIdFromOCMAddress(req.Sender)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "error with remote sender", err)
		return
	}
	owner, err := GetUserIdFromOCMAddress(req.Owner)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "error with remote owner", err)
		return
	}

	// extract the client IP (or the proxied one) from the request and validate it against the allowed providers
	// TODO(lopresti) this should rather be replaced with signed requests as per more recent OCM specifications
	senderIP, err := utils.GetClientIP(r)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, fmt.Sprintf("error retrieving client IP from request: %s", r.RemoteAddr), err)
		return
	}
	providerInfo := ocmprovider.ProviderInfo{
		Domain: sender.Idp,
		Services: []*ocmprovider.Service{
			{
				Host: senderIP,
			},
		},
	}
	providerAllowedResp, err := h.gatewayClient.IsProviderAllowed(ctx, &ocmprovider.IsProviderAllowedRequest{
		Provider: &providerInfo,
	})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error sending a grpc isProviderAllowed request", err)
		return
	}
	if providerAllowedResp.Status.Code != rpc.Code_CODE_OK {
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, "provider not authorized", errors.New(providerAllowedResp.Status.Message))
		return
	}

	shareWith, err := GetUserIdFromOCMAddress(req.ShareWith)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "error with shareWith user", err)
		return
	}

	userRes, err := h.gatewayClient.GetUser(ctx, &userpb.GetUserRequest{
		UserId: &userpb.UserId{OpaqueId: shareWith.OpaqueId}, SkipFetchingUserGroups: true,
	})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error searching recipient", err)
		return
	}
	if userRes.Status.Code != rpc.Code_CODE_OK {
		reqres.WriteError(w, r, reqres.APIErrorNotFound, "user not found", errors.New(userRes.Status.Message))
		return
	}

	// When an auto-accept whitelist is configured it acts as a mandatory allowlist:
	// shares from a provider that does not match are rejected outright and never stored.
	// An empty whitelist disables this gate, preserving the default "accept all" behavior.
	// A sender that is already an accepted user of the recipient is let through
	// regardless of the whitelist (they went through a prior invitation flow).
	if len(h.autoAcceptProviders) > 0 && !h.matchesAutoAccept(sender.Idp) && !h.isAcceptedUser(ctx, userRes.User, sender) {
		reqres.WriteError(w, r, reqres.APIErrorUnauthenticated, "sender provider not in the auto-accept whitelist", nil)
		return
	}

	protocols, legacy, err := getAndResolveProtocols(ctx, req.Protocols, req.ResourceType, sender.Idp)
	if err != nil || len(protocols) == 0 {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, "error with protocols payload", err)
		return
	}

	if legacy && req.ResourceType == "file" {
		// in case of legacy OCM v1.0 shares, we have to PROPFIND the remote resource to check the type,
		// because remote systems such as Nextcloud may send "file" even if the resource is a folder.
		c := gowebdav.NewClient(protocols[0].GetWebdavOptions().Uri, "", "")
		c.SetHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(protocols[0].GetWebdavOptions().SharedSecret+":")))
		target, err := c.Stat("")
		if err != nil {
			log.Info().Err(err).Str("endpoint", protocols[0].GetWebdavOptions().Uri).Msg("error stating remote resource, assuming file")
		} else if target.IsDir() {
			req.ResourceType = "folder"
		}
	}

	createShareReq := &ocmincoming.CreateOCMIncomingShareRequest{
		Description:        req.Description,
		Name:               req.Name,
		ResourceId:         req.ProviderID,
		Owner:              owner,
		Sender:             sender,
		ShareWith:          userRes.User.Id,
		SharedResourceType: getResourceTypeFromOCMRequest(req.ResourceType),
		RecipientType:      getOCMShareType(req.ShareType),
		Protocols:          protocols,
	}

	if req.Expiration != 0 {
		createShareReq.Expiration = &types.Timestamp{
			Seconds: req.Expiration,
		}
	}

	log.Info().Str("resource_id", req.ProviderID).Str("sender", req.Sender).Str("resource_type", req.ResourceType).Msg("CreateOCMIncomingShare payload")
	createShareResp, err := h.gatewayClient.CreateOCMIncomingShare(ctx, createShareReq)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error creating ocm share", err)
		return
	}

	if createShareResp.Status.Code != rpc.Code_CODE_OK {
		// TODO: define errors in the cs3apis
		reqres.WriteError(w, r, reqres.APIErrorServerError, "could not create ocm share", errors.New(createShareResp.Status.Message))
		return
	}

	// Shares from a whitelisted provider arrive without a prior invitation flow, so
	// their remote sender/owner are not yet known as accepted users and a later
	// resolution of them would fail. Register them locally now, while the payload still
	// carries their display names. This is auxiliary: any failure is logged but does
	// not fail the share creation.
	if h.matchesAutoAccept(sender.Idp) {
		h.autoRegisterRemoteUsers(ctx, userRes.User, sender, req.SenderDisplayName, owner, req.OwnerDisplayName)
	}

	response := map[string]any{}
	if h.exposeRecipientDisplayName {
		response["recipientDisplayName"] = userRes.User.DisplayName
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}

// autoRegisterRemoteUsers registers the share's remote sender and owner as accepted
// users of the recipient, so they can be resolved later without a prior invitation
// flow. The owner is skipped when it is the same as the sender.
func (h *sharesHandler) autoRegisterRemoteUsers(ctx context.Context, recipient *userpb.User, sender *userpb.UserId, senderDisplayName string, owner *userpb.UserId, ownerDisplayName string) {
	log := appctx.GetLogger(ctx)

	h.registerAcceptedUser(ctx, recipient, &userpb.User{
		Id:          sender,
		DisplayName: senderDisplayName,
	})

	if owner.OpaqueId == sender.OpaqueId && owner.Idp == sender.Idp {
		return
	}
	if owner.OpaqueId == "" {
		log.Debug().Msg("auto-register: skipping empty owner")
		return
	}
	h.registerAcceptedUser(ctx, recipient, &userpb.User{
		Id:          owner,
		DisplayName: ownerDisplayName,
	})
}

// registerAcceptedUser adds remoteUser to the recipient's accepted users, unless it is
// already known. It reuses the existing invite API: a local invite token is minted on
// behalf of the recipient (via machine-auth impersonation) and immediately accepted,
// which inserts the (recipient, remoteUser) relation. The token is never sent remotely
// and simply expires. All failures are logged and swallowed.
func (h *sharesHandler) registerAcceptedUser(ctx context.Context, recipient *userpb.User, remoteUser *userpb.User) {
	log := appctx.GetLogger(ctx)

	// skip if the remote user is already an accepted user of the recipient
	if h.isAcceptedUser(ctx, recipient, remoteUser.Id) {
		log.Debug().Str("remote_user", remoteUser.Id.OpaqueId).Msg("auto-register: remote user already accepted, skipping")
		return
	}

	// impersonate the recipient so the minted invite token is owned by them
	recipientCtx, err := h.impersonate(ctx, recipient)
	if err != nil {
		log.Error().Err(err).Str("recipient", recipient.Id.OpaqueId).Msg("auto-register: error impersonating recipient")
		return
	}

	tokenRes, err := h.gatewayClient.GenerateInviteToken(recipientCtx, &invitepb.GenerateInviteTokenRequest{
		Description: "auto-accept for received OCM share",
	})
	if err != nil || tokenRes.Status.Code != rpc.Code_CODE_OK {
		log.Error().Err(err).Interface("status", tokenRes.GetStatus()).Msg("auto-register: error generating invite token")
		return
	}

	acceptRes, err := h.gatewayClient.AcceptInvite(ctx, &invitepb.AcceptInviteRequest{
		InviteToken: tokenRes.InviteToken,
		RemoteUser:  remoteUser,
	})
	if err != nil {
		log.Error().Err(err).Msg("auto-register: error accepting invite")
		return
	}
	if acceptRes.Status.Code != rpc.Code_CODE_OK && acceptRes.Status.Code != rpc.Code_CODE_ALREADY_EXISTS {
		log.Error().Interface("status", acceptRes.Status).Msg("auto-register: error accepting invite")
		return
	}
	log.Info().Str("recipient", recipient.Id.OpaqueId).Str("remote_user", remoteUser.Id.OpaqueId).Msg("auto-registered remote user as accepted user")
}

// impersonate returns a context authenticated as the given user via machine auth,
// for invoking protected calls (e.g. GenerateInviteToken) on their behalf.
func (h *sharesHandler) impersonate(ctx context.Context, user *userpb.User) (context.Context, error) {
	authRes, err := h.gatewayClient.Authenticate(ctx, &gateway.AuthenticateRequest{
		Type:         "machine",
		ClientId:     user.Username,
		ClientSecret: h.machineSecret,
	})
	if err != nil {
		return nil, err
	}
	if authRes.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.New("error impersonating user: " + authRes.Status.Message)
	}

	userCtx := appctx.ContextSetToken(ctx, authRes.Token)
	userCtx = appctx.ContextSetUser(userCtx, authRes.User)
	userCtx = metadata.AppendToOutgoingContext(userCtx, appctx.TokenHeader, authRes.Token)
	return userCtx, nil
}

func getCreateShareRequest(r *http.Request) (*NewShareRequest, error) {
	var req NewShareRequest
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil && contentType == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, errors.Wrap(err, "malformed OCM /shares request")
		}
	} else {
		return nil, errors.New("malformed OCM /shares request payload")
	}
	// validate the request
	if err := validate.Struct(req); err != nil {
		return nil, err
	}
	// Protocols are interface-backed, so validate the decoded protocol payloads
	// explicitly before we create or persist a received share.
	if err := req.Protocols.Validate(); err != nil {
		return nil, err
	}
	return &req, nil
}

func getResourceTypeFromOCMRequest(t string) ocm.SharedResourceType {
	switch t {
	case "file":
		return ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE
	case "folder":
		return ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER
	case "ro-crate":
		// RO-Crate resources are processed as embedded payloads
		return ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED
	// Similarly, JSON resources could be processed as embedded payloads, should we need them
	//case "json":
	//	return ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED
	default:
		return ocm.SharedResourceType_SHARE_RESOURCE_TYPE_INVALID
	}
}

func getOCMShareType(st string) ocm.RecipientType {
	switch st {
	case "user":
		return ocm.RecipientType_RECIPIENT_TYPE_USER
	case "group":
		return ocm.RecipientType_RECIPIENT_TYPE_GROUP
	default:
		// legacy OCM endpoints used to not send the recipient type, so default to `user` if not provided
		return ocm.RecipientType_RECIPIENT_TYPE_USER
	}
}

func getAndResolveProtocols(ctx context.Context, p Protocols, resType string, ownerServer string) (protos []*ocm.Protocol, legacy bool, err error) {
	protos = make([]*ocm.Protocol, 0, len(p))
	legacy = false

	// discover remote resource types
	ocmRTs, ocmEndpoint, err := discoverOcmResourceTypes(ctx, ownerServer)
	if err != nil {
		return nil, false, errors.Wrap(err, "error discovering remote OCM resource types")
	}

	for _, data := range p {
		var uri string
		var protoInfo any
		var ok bool
		ocmProto := data.ToOCMProtocol()
		protocolName := GetProtocolName(data)
		for _, rt := range ocmRTs {
			if rt.Name == resType {
				if protoInfo, ok = rt.Protocols[protocolName]; ok {
					break
				}
			}
		}
		if protoInfo == nil {
			return nil, false, fmt.Errorf("the remote OCM server does not advertise the %s OCM protocol for %s", protocolName, resType)
		}

		switch protocolName {
		case "webdav":
			uri = ocmProto.GetWebdavOptions().Uri
		case "webapp":
			uri = ocmProto.GetWebappOptions().Uri
		case "embedded":
			protos = append(protos, ocmProto)
			continue
		default:
			return nil, false, fmt.Errorf("unsupported OCM protocol: %s", protocolName)
		}

		// Absolute URIs should already be clean sender-owned endpoints. Validate
		// again here so malformed values fail before any discovery-based rewriting.
		if err := validateProtocolURI(protocolName, uri); err != nil {
			return nil, false, err
		}

		// If the `uri` contains a hostname, use it as is
		u, err := url.Parse(uri)
		if err != nil {
			return nil, false, errors.Wrapf(err, "error parsing protocol URI '%s'", uri)
		}
		if u.Host != "" {
			protos = append(protos, ocmProto)
			continue
		}

		// otherwise use as endpoint the owner's server from the payload, if found:
		// this can be accepted for `webdav` legacy shares where the `uri` is actually a
		// (relative) path or missing
		if protocolName != "webdav" {
			return nil, false, fmt.Errorf("invalid protocol URI: missing host for protocol '%s'", protocolName)
		}
		protoRoot, ok := protoInfo.(string)
		if !ok {
			return nil, false, fmt.Errorf("missing host in URI '%s' and root webdav path not advertised by the remote OCM server", uri)
		}

		u, err = url.Parse(ocmEndpoint)
		if err != nil {
			return nil, false, errors.Wrapf(err, "error parsing remote OCM endpoint '%s'", ocmEndpoint)
		}
		if strings.HasPrefix(uri, "/") {
			u.Path = uri
		} else if uri == "" {
			// case of an OCM v1.0 share with no uri, use root
			u.Path = protoRoot
			legacy = true
		} else {
			// relative uri: prepend the found protocol root
			u.Path = filepath.Join(protoRoot, uri)
		}
		ocmProto.GetWebdavOptions().Uri = u.String()
		protos = append(protos, ocmProto)
	}

	return protos, legacy, nil
}

func discoverOcmResourceTypes(ctx context.Context, ownerServer string) ([]wellknown.ResourceTypes, string, error) {
	ocmClient := NewClient(time.Duration(10)*time.Second, true)
	ocmCaps, err := ocmClient.Discover(ctx, ownerServer)
	if err != nil {
		return nil, "", err
	}

	return ocmCaps.ResourceTypes, ocmCaps.Endpoint, nil
}
