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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ocmshare "github.com/cs3org/reva/v3/pkg/ocm/share"
	utils "github.com/cs3org/reva/v3/pkg/utils"
)

// In this file we group the definitions of the OCM payloads according to the official specs
// at https://github.com/cs3org/OCM-API/blob/develop/spec.yaml

// InviteAcceptedRequest contains the payload of an OCM /invite-accepted request.
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1invite-accepted/post
type InviteAcceptedRequest struct {
	UserID            string `json:"userID" validate:"required"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	RecipientProvider string `json:"recipientProvider"`
	Token             string `json:"token"`
}

// RemoteUser contains the remote user's information both when sending an /invite-accepted call and when sending back a response to /invite-accepted
type RemoteUser struct {
	UserID string `json:"userID"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

func (r *InviteAcceptedRequest) toJSON() ([]byte, error) {
	return json.Marshal(&r)
}

// DirectoryService represents a directory service listing per OCM spec Appendix C.
type DirectoryService struct {
	Federation string                   `json:"federation"`
	Servers    []DirectoryServiceServer `json:"servers"`
}

// DirectoryServiceServer represents a single OCM server in a directory service.
type DirectoryServiceServer struct {
	DisplayName string `json:"displayName"`
	URL         string `json:"url"`
	// Added after discovery, not in raw response
	InviteAcceptDialog string `json:"inviteAcceptDialog,omitempty"`
}

// NewShareRequest contains the payload of an OCM /share request.
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1shares/post
type NewShareRequest struct {
	ShareWith         string    `json:"shareWith"         validate:"required"`                  // identifier of the recipient of the share
	Name              string    `json:"name"              validate:"required"`                  // name of the resource
	Description       string    `json:"description"`                                            // (optional) description of the resource
	ProviderID        string    `json:"providerId"        validate:"required"`                  // unique identifier of the resource at provider side
	Owner             string    `json:"owner"             validate:"required"`                  // unique identifier of the owner at provider side
	Sender            string    `json:"sender"            validate:"required"`                  // unique indentifier of the user who wants to share the resource at provider side
	OwnerDisplayName  string    `json:"ownerDisplayName"`                                       // display name of the owner of the resource
	SenderDisplayName string    `json:"senderDisplayName"`                                      // dispay name of the user who wants to share the resource
	Code              string    `json:"code"`                                                   // nonce to be exchanged for a bearer token (not implemented for now)
	ShareType         string    `json:"shareType"         validate:"required,oneof=user group"` // recipient share type (user or group)
	ResourceType      string    `json:"resourceType"      validate:"required,oneof=file folder ro-crate"`
	Expiration        uint64    `json:"expiration"`
	Protocols         Protocols `json:"protocol"          validate:"required"`
}

func (r *NewShareRequest) toJSON() ([]byte, error) {
	return json.Marshal(&r)
}

// NewShareResponse is the response returned when creating a new share.
type NewShareResponse struct {
	RecipientDisplayName string `json:"recipientDisplayName"`
}

// Protocols is the list of OCM protocols.
type Protocols []Protocol

// Protocol represents the way of access the resource
// in the OCM share.
type Protocol interface {
	// ToOCMProtocol converts the protocol to a CS3API OCM `Protocol` struct
	ToOCMProtocol() *ocm.Protocol
}

// protocols supported by the OCM API

// WebDAV contains the parameters for the WebDAV protocol.
type WebDAV struct {
	SharedSecret string   `json:"sharedSecret" validate:"required"`
	Permissions  []string `json:"permissions"  validate:"required,dive,required,oneof=read write share"`
	Requirements []string `json:"requirements,omitempty"`
	URI          string   `json:"uri"          validate:"required"`
}

// ToOCMProtocol convert the protocol to a ocm Protocol struct.
func (w *WebDAV) ToOCMProtocol() *ocm.Protocol {
	perms := &ocm.SharePermissions{
		Permissions: &providerv1beta1.ResourcePermissions{},
	}
	for _, p := range w.Permissions {
		switch p {
		case "read":
			perms.Permissions.GetPath = true
			perms.Permissions.InitiateFileDownload = true
			perms.Permissions.ListContainer = true
			perms.Permissions.Stat = true
		case "write":
			perms.Permissions.InitiateFileUpload = true
		case "share":
			perms.Reshare = true
		}
	}

	return ocmshare.NewWebDAVProtocol(w.URI, w.SharedSecret, perms, w.Requirements)
}

// Webapp contains the parameters for the Webapp protocol.
type Webapp struct {
	URI          string `json:"uri" validate:"required"`
	ViewMode     string `json:"viewMode"    validate:"required,dive,required,oneof=view read write"`
	SharedSecret string `json:"sharedSecret"`
}

// ToOCMProtocol convert the protocol to a ocm Protocol struct.
func (w *Webapp) ToOCMProtocol() *ocm.Protocol {
	return ocmshare.NewWebappProtocol(w.URI, utils.GetAppViewMode(w.ViewMode))
}

// Datatx contains the parameters for the Datatx protocol.
type Datatx struct {
	SharedSecret string `json:"sharedSecret" validate:"required"`
	SourceURI    string `json:"srcUri"       validate:"required"`
	Size         uint64 `json:"size"         validate:"required"`
}

// ToOCMProtocol convert the protocol to a ocm Protocol struct.
func (w *Datatx) ToOCMProtocol() *ocm.Protocol {
	return ocmshare.NewTransferProtocol(w.SourceURI, w.SharedSecret, w.Size)
}

// Embedded contains the parameters for the Embedded protocol.
type Embedded struct {
	Payload json.RawMessage `json:"payload" validate:"required"`
}

// ToOCMProtocol convert the protocol to a ocm Protocol struct.
func (w *Embedded) ToOCMProtocol() *ocm.Protocol {
	return ocmshare.NewEmbeddedProtocol(string(w.Payload))
}

var protocolImpl = map[string]reflect.Type{
	"webdav":   reflect.TypeOf(WebDAV{}),
	"webapp":   reflect.TypeOf(Webapp{}),
	"datatx":   reflect.TypeOf(Datatx{}),
	"embedded": reflect.TypeOf(Embedded{}),
}

// UnmarshalJSON implements the Unmarshaler interface.
func (p *Protocols) UnmarshalJSON(data []byte) error {
	var prot map[string]json.RawMessage
	if err := json.Unmarshal(data, &prot); err != nil {
		return err
	}

	*p = []Protocol{}

	for name, d := range prot {
		var res Protocol

		if name == "name" {
			continue
		}
		if name == "options" {
			var opt map[string]any
			if err := json.Unmarshal(d, &opt); err != nil {
				return fmt.Errorf("malformed protocol options %s", d)
			}
			if len(opt) > 0 {
				// This is an OCM 1.0 payload: parse the secret and assume max
				// permissions, as in the OCM 1.0 model the remote server would check
				// (and would not tell to the sharee!) which permissions are enabled
				// on the share. Also, in this case the URL has to be resolved via
				// discovery, see shares.go.
				ss, ok := opt["sharedSecret"].(string)
				if !ok {
					return fmt.Errorf("missing sharedSecret from options %s", d)
				}
				res = &WebDAV{
					SharedSecret: ss,
					Permissions:  []string{"read", "write", "share"},
					URI:          "",
				}
				*p = append(*p, res)
			}
			continue
		}
		ctype, ok := protocolImpl[name]
		if !ok {
			return fmt.Errorf("protocol %s not recognised", name)
		}
		res = reflect.New(ctype).Interface().(Protocol)
		if err := json.Unmarshal(d, &res); err != nil {
			return err
		}

		*p = append(*p, res)
	}
	return nil
}

// MarshalJSON implements the Marshaler interface.
func (p Protocols) MarshalJSON() ([]byte, error) {
	if len(p) == 0 {
		return nil, errors.New("no protocol defined")
	}
	d := make(map[string]any)
	for _, prot := range p {
		d[GetProtocolName(prot)] = prot
	}
	// fill in the OCM v1.0 properties: we only create OCM 1.1+ payloads,
	// irrespective from the capabilities of the remote server.
	d["name"] = "multi"
	d["options"] = map[string]any{}
	return json.Marshal(d)
}

// GetProtocolName returns the name of the protocol by reflection.
func GetProtocolName(p Protocol) string {
	n := reflect.TypeOf(p).String()
	s := strings.Split(n, ".")
	return strings.ToLower(s[len(s)-1])
}

// GetUserIdFromOCMAddress parses an OCM address identifier in the form <id>@<provider>
// according to the specifications, see https://github.com/cs3org/OCM-API/blob/develop/IETF-RFC.md#terms
func GetUserIdFromOCMAddress(user string) (*userpb.UserId, error) {
	last := strings.LastIndex(user, "@")
	if last == -1 {
		return nil, errors.New("not in the form <id>@<provider>")
	}

	id, idp := user[:last], user[last+1:]
	if id == "" {
		return nil, errors.New("id cannot be empty")
	}
	if idp == "" {
		return nil, errors.New("provider cannot be empty")
	}
	idp = strings.TrimPrefix(idp, "https://") // strip off leading scheme if present (despite being not OCM compliant). This is the case in Nextcloud and oCIS

	return &userpb.UserId{
		OpaqueId: id,
		Idp:      idp,
		// a remote user is a federated account for the local system
		Type: userpb.UserType_USER_TYPE_FEDERATED,
	}, nil
}
