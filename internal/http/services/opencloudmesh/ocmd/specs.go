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
	"net/url"
	"reflect"
	"slices"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ocmshare "github.com/cs3org/reva/v3/pkg/ocm/share"
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
	ShareWith         string    `json:"shareWith"         validate:"required"`                             // identifier of the recipient of the share
	Name              string    `json:"name"              validate:"required"`                             // name of the resource
	Description       string    `json:"description"`                                                       // (optional) description of the resource
	ProviderID        string    `json:"providerId"        validate:"required"`                             // unique identifier of the resource at provider side
	Owner             string    `json:"owner"             validate:"required"`                             // unique identifier of the owner at provider side
	Sender            string    `json:"sender"            validate:"required"`                             // unique indentifier of the user who wants to share the resource at provider side
	OwnerDisplayName  string    `json:"ownerDisplayName"`                                                  // display name of the owner of the resource
	SenderDisplayName string    `json:"senderDisplayName"`                                                 // dispay name of the user who wants to share the resource
	Code              string    `json:"code"`                                                              // nonce to be exchanged for a bearer token (not implemented for now)
	ShareType         string    `json:"shareType"         validate:"required,oneof=user group federation"` // recipient share type (user or group)
	ResourceType      string    `json:"resourceType"      validate:"required"`
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

var validWebDAVPermissions = map[string]struct{}{
	"read":  {},
	"write": {},
	"share": {},
}

var validWebDAVRequirements = map[string]struct{}{
	"must-exchange-token": {},
}

var validWebappPermissions = map[string]struct{}{
	"view":  {},
	"read":  {},
	"write": {},
	"share": {},
}

var validWebappRequirements = map[string]struct{}{
	"must-use-mfa":        {},
	"must-exchange-token": {},
}

var validWebappTargets = map[string]struct{}{
	"blank":  {},
	"iframe": {},
}

// protocols supported by the OCM API

// WebDAV contains the parameters for the WebDAV protocol.
type WebDAV struct {
	SharedSecret string   `json:"sharedSecret" validate:"required"`
	AccessTypes  []string `json:"accessTypes,omitempty" validate:"dive,required,oneof=remote datatx"` // defaults to ["remote"]
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
	accTypes := []ocm.AccessType{}
	for _, at := range w.AccessTypes {
		switch at {
		case "remote":
			accTypes = append(accTypes, ocm.AccessType_ACCESS_TYPE_REMOTE)
		case "datatx":
			accTypes = append(accTypes, ocm.AccessType_ACCESS_TYPE_DATATX)
		}
	}
	if len(accTypes) == 0 {
		accTypes = append(accTypes, ocm.AccessType_ACCESS_TYPE_REMOTE)
	}
	return ocmshare.NewWebDAVProtocol(w.URI, w.SharedSecret, perms, accTypes, w.Requirements)
}

// Webapp contains the parameters for the Webapp protocol.
type Webapp struct {
	URI          string   `json:"uri" validate:"required"`
	SharedSecret string   `json:"sharedSecret" validate:"required"`
	Permissions  []string `json:"permissions" validate:"required,min=1,dive,required,oneof=view read write share"`
	Requirements []string `json:"requirements" validate:"required,min=1"`
	Targets      []string `json:"targets" validate:"required,min=1,dive,required,oneof=blank iframe"`
	AppName      string   `json:"appName,omitempty"`
	AppIconHint  string   `json:"appIconHint,omitempty"`
	MediaTypes   []string `json:"mediaTypes,omitempty"`
}

// ToOCMProtocol convert the protocol to a ocm Protocol struct.
func (w *Webapp) ToOCMProtocol() *ocm.Protocol {
	perms := &providerv1beta1.ResourcePermissions{}
	for _, p := range w.Permissions {
		switch p {
		case "view":
			perms.GetPath = true
			perms.ListContainer = true
			perms.Stat = true
		case "read":
			perms.GetPath = true
			perms.ListContainer = true
			perms.Stat = true
			perms.InitiateFileDownload = true
		case "write":
			perms.InitiateFileUpload = true
		case "share":
			perms.AddGrant = true
		}
	}
	return ocmshare.NewWebappProtocol(w.URI, w.SharedSecret, perms, w.Requirements, w.Targets, w.AppName, w.AppIconHint, w.MediaTypes)
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
	"webdav":   reflect.TypeFor[WebDAV](),
	"webapp":   reflect.TypeFor[Webapp](),
	"embedded": reflect.TypeFor[Embedded](),
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
		ptype, ok := protocolImpl[name]
		if !ok {
			return fmt.Errorf("protocol %s not recognised", name)
		}
		res = reflect.New(ptype).Interface().(Protocol)
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

// Validate applies the protocol checks that the generic request validator cannot
// express once the payload has been unmarshaled into the Protocol interface.
func (p Protocols) Validate() error {
	if len(p) == 0 {
		return errors.New("missing protocol definition")
	}

	for _, protocol := range p {
		switch data := protocol.(type) {
		case *WebDAV:
			if err := validateSharedProtocolFields("webdav", data.SharedSecret, data.Permissions, validWebDAVPermissions, data.Requirements, validWebDAVRequirements, data.URI); err != nil {
				return err
			}
		case *Webapp:
			if err := validateSharedProtocolFields("webapp", data.SharedSecret, data.Permissions, validWebappPermissions, data.Requirements, validWebappRequirements, data.URI); err != nil {
				return err
			}
			if len(data.Targets) == 0 {
				return errors.New("protocol webapp missing targets")
			}
			if err := validateVocabulary("webapp", "target", data.Targets, validWebappTargets); err != nil {
				return err
			}
			// The spec mandates that webapp requirements include `must-exchange-token`.
			if !slices.Contains(data.Requirements, "must-exchange-token") {
				return errors.New("protocol webapp requirements must include must-exchange-token")
			}
		}
	}

	return nil
}

// validateSharedProtocolFields applies the checks common to the webdav and
// webapp protocols: a shared secret must be present, at least one permission
// must be set and drawn from the protocol's vocabulary, every requirement must
// be recognised, and the URI must be well-formed.
func validateSharedProtocolFields(name, sharedSecret string, permissions []string, validPermissions map[string]struct{}, requirements []string, validRequirements map[string]struct{}, uri string) error {
	if sharedSecret == "" {
		return fmt.Errorf("protocol %s missing sharedSecret", name)
	}
	if len(permissions) == 0 {
		return fmt.Errorf("protocol %s missing permissions", name)
	}
	// Reject unknown permission vocabularies here so they do not degrade into
	// an empty CS3 permission set later in ToOCMProtocol.
	if err := validateVocabulary(name, "permission", permissions, validPermissions); err != nil {
		return err
	}
	if err := validateVocabulary(name, "requirement", requirements, validRequirements); err != nil {
		return err
	}
	return validateProtocolURI(name, uri)
}

// validateVocabulary checks that every value belongs to the allowed set,
// reporting the first one that does not.
func validateVocabulary(protocolName, kind string, values []string, valid map[string]struct{}) error {
	for _, value := range values {
		if _, ok := valid[value]; !ok {
			return fmt.Errorf("protocol %s has unsupported %s %q", protocolName, kind, value)
		}
	}
	return nil
}

// Absolute protocol URIs should already be fully usable sender endpoints. Catch
// malformed values such as double-scheme hosts before they are stored or resolved.
func validateProtocolURI(protocolName, uri string) error {
	if uri == "" {
		return nil
	}

	parsedURI, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("protocol %s has invalid uri %q: %w", protocolName, uri, err)
	}
	if parsedURI.Host == "" {
		return nil
	}
	if parsedURI.Scheme != "http" && parsedURI.Scheme != "https" {
		return fmt.Errorf("protocol %s has unsupported absolute uri scheme %q", protocolName, parsedURI.Scheme)
	}
	if parsedURI.Host == "http:" || parsedURI.Host == "https:" ||
		strings.Contains(parsedURI.Host, "://") ||
		strings.HasPrefix(parsedURI.Path, "//http://") ||
		strings.HasPrefix(parsedURI.Path, "//https://") {
		return fmt.Errorf("protocol %s has malformed absolute uri %q", protocolName, uri)
	}

	return nil
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

// TrimOCMScheme removes a leading http(s):// scheme from an OCM host string.
// OCM Addresses are not URIs and MUST NOT carry a scheme, but some servers
// (Nextcloud, oCIS, OpenCloud) include one; we strip it defensively.
func TrimOCMScheme(host string) string {
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	return host
}

// NormalizeRemoteUserID returns the bare OCM identifier for a remote user.
//
// Per the OCM spec the invite `userID` MUST be the bare identifier of the user
// at their OCM Server, and the host travels separately in `recipientProvider`.
// Some non-conformant servers append the host to `userID` anyway (oCIS sends
// "id@host", OpenCloud sends "id@https://host"). If stored verbatim, CERNBox
// keeps that qualified string as the OpaqueId and later re-appends the provider
// domain when building `shareWith`, producing "id@host@host" (or with a scheme),
// which the receiver cannot resolve to a local user.
//
// We strip a trailing "@<provider>" suffix ONLY when it matches the already-known
// provider domain, repeating to collapse accidental double-qualification. A
// spec-conformant identifier that legitimately contains '@' (e.g. an email local
// part such as "a@b.org" belonging to a different provider) is left untouched.
func NormalizeRemoteUserID(userID, providerDomain string) string {
	host := TrimOCMScheme(providerDomain)
	if host == "" {
		return userID
	}
	for {
		uid, err := GetUserIdFromOCMAddress(userID)
		if err != nil || !strings.EqualFold(TrimOCMScheme(uid.Idp), host) {
			return userID
		}
		userID = uid.OpaqueId
	}
}

// FormatOCMUser renders a CS3 user id as an OCM Address "<opaque-id>@<host>".
// It strips any scheme from the host and collapses a redundant, self-referential
// provider suffix already present in the opaque id, so it never emits the
// malformed "id@host@host" form even if a non-conformant peer polluted storage.
func FormatOCMUser(u *userpb.UserId) string {
	host := TrimOCMScheme(u.Idp)
	opaque := NormalizeRemoteUserID(u.OpaqueId, host)
	return fmt.Sprintf("%s@%s", opaque, host)
}
