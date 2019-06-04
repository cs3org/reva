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

package ocssvc

import (
	"net/http"

	publicsharev0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshare/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	shareregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/shareregistry/v0alpha"
	sharetypespb "github.com/cs3org/go-cs3apis/cs3/sharetypes"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"

	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
	"google.golang.org/grpc"
)

// SharesHandler implements the ownCloud sharing API
type SharesHandler struct {
	shareRegistrySvc string
	conn             *grpc.ClientConn
	client           shareregistryv0alphapb.ShareRegistryServiceClient
}

func (h *SharesHandler) init(c *Config) {
	h.shareRegistrySvc = ":9999" // TODO(jfd) fixme read from config
}

func (h *SharesHandler) getConn() (*grpc.ClientConn, error) {
	if h.conn != nil {
		return h.conn, nil
	}

	conn, err := grpc.Dial(h.shareRegistrySvc, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (h *SharesHandler) getClient() (shareregistryv0alphapb.ShareRegistryServiceClient, error) {
	if h.client != nil {
		return h.client, nil
	}

	conn, err := h.getConn()
	if err != nil {
		return nil, err
	}
	h.client = shareregistryv0alphapb.NewShareRegistryServiceClient(conn)
	return h.client, nil
}

func (h *SharesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	var head string
	head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)

	log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")

	switch head {
	case "shares":
		// TODO PUT vs GET
		h.listShares(w, r)
	default:
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

func (h *SharesHandler) listShares(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	client, err := h.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &shareregistryv0alphapb.ListShareProvidersRequest{}
	res, err := client.ListShareProviders(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc ListShareProviders request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if res.Status.Code != rpcpb.Code_CODE_OK {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	filters := []*usershareproviderv0alphapb.ListSharesRequest_Filter{}

	path := r.URL.Query().Get("path")
	if path != "" {
		filters = append(filters, &usershareproviderv0alphapb.ListSharesRequest_Filter{
			Type: usershareproviderv0alphapb.ListSharesRequest_Filter_LIST_SHARES_REQUEST_FILTER_TYPE_RESOURCE_ID,
			Term: &usershareproviderv0alphapb.ListSharesRequest_Filter_ResourceId{
				ResourceId: &storageproviderv0alphapb.ResourceId{
					StorageId: "", // TODO(jfd) lookup correct storage, for now this always uses the configured storage driver, maybe the combined storage can delegate this?
					OpaqueId:  path,
				},
			},
		})
	}

	shares := []*ShareData{}

	for _, p := range res.Providers {
		// query this provider
		pConn, err := grpc.Dial(p.Address, grpc.WithInsecure())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		switch p.ShareType {
		case sharetypespb.ShareType_SHARE_TYPE_USER:
			pClient := usershareproviderv0alphapb.NewUserShareProviderServiceClient(pConn)
			req := &usershareproviderv0alphapb.ListSharesRequest{
				Filters: filters,
			}
			res, err := pClient.ListShares(ctx, req)
			if err != nil {
				log.Error().Err(err).Str("address", p.Address).Msg("error sending a grpc list shares request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if res.Status.Code != rpcpb.Code_CODE_OK {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			for _, s := range res.Share {
				sd := &ShareData{
					ID: s.Id.OpaqueId,
				}
				shares = append(shares, sd)
			}
		case sharetypespb.ShareType_SHARE_TYPE_PUBLIC_LINK:
			pClient := publicsharev0alphapb.NewPublicShareProviderServiceClient(pConn)
			req := &publicsharev0alphapb.ListPublicSharesRequest{}
			res, err := pClient.ListPublicShares(ctx, req)
			if err != nil {
				log.Error().Err(err).Msg("error sending a grpc stat request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if res.Status.Code != rpcpb.Code_CODE_OK {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			for _, s := range res.Share {
				sd := &ShareData{
					ID: s.Id.OpaqueId,
				}
				shares = append(shares, sd)
			}
		}

	}
	// get shares registry
	// get share provider

	res2 := &Response{
		OCS: &Payload{
			Meta: MetaOK,
			Data: SharesData{
				Shares: shares,
			},
		},
	}

	err = WriteOCSResponse(w, r, res2)
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing ocs response")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// SharesData holds a list of share data
type SharesData struct {
	Shares []*ShareData `json:"element" xml:"element"`
}

// ShareData holds share data
type ShareData struct {
	ID                   string `json:"id" xml:"id"`
	ShareType            string `json:"share_type" xml:"share_type"`
	DisplaynameOwner     string `json:"displayname_owner" xml:"displayname_owner"`
	Permissions          string `json:"permissions" xml:"permissions"`
	STime                string `json:"stime" xml:"stime"`
	Parent               string `json:"parent" xml:"parent"`
	Expiration           string `json:"expiration" xml:"expiration"`
	Token                string `json:"token" xml:"token"`
	UIDFileOwner         string `json:"uid_file_owner" xml:"uid_file_owner"`
	DisplaynameFileOwner string `json:"displayname_file_owner" xml:"displayname_file_owner"`
	Path                 string `json:"path" xml:"path"`
	ItemType             string `json:"item_type" xml:"item_type"`
	MimeType             string `json:"mimetype" xml:"mimetype"`
	StorageID            string `json:"storage_id" xml:"storage_id"`
	Storage              string `json:"storage" xml:"storage"`
	ItemSource           string `json:"item_source" xml:"item_source"`
	FileSource           string `json:"file_source" xml:"file_source"`
	FileParent           string `json:"file_parent" xml:"file_parent"`
	FileTarget           string `json:"file_target" xml:"file_target"`
	ShareWith            string `json:"share_with" xml:"share_with"`
	ShareWithDisplayname string `json:"share_with_displayname" xml:"share_with_displayname"`
	MailSend             string `json:"mail_send" xml:"mail_send"`
}
