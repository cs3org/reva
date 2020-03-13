// Copyright 2018-2020 CERN
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
	"encoding/json"
	"fmt"
	"net/url"

	"go.uber.org/zap"
)

type apiErrorCode string

const (
	apiErrorNotFound         apiErrorCode = "RESOURCE_NOT_FOUND"
	apiErrorUnauthenticated  apiErrorCode = "UNAUTHENTICATED"
	apiErrorUntrustedService apiErrorCode = "UNTRUSTED_SERVICE"
	apiErrorUnimplemented    apiErrorCode = "FUNCTION_NOT_IMPLEMENTED"
	apiErrorInvalidParameter apiErrorCode = "INVALID_PARAMETER"
	apiErrorProviderError    apiErrorCode = "PROVIDER_ERROR"
)

func newAPIError(code apiErrorCode) *apiError {
	return &apiError{Code: code}
}

type apiError struct {
	Code    apiErrorCode `json:"code"`
	Message string       `json:"message"`
}

func (e *apiError) WithMessage(msg string) *apiError {
	e.Message = msg
	return e
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *apiError) JSON() []byte {
	b, _ := json.MarshalIndent(e, "", "    ")
	return b
}

type share struct {
	ShareWith         string        `json:"shareWith"`
	Name              string        `json:"name"`
	Description       string        `json:"description"`
	ProviderID        string        `json:"providerId"`
	Owner             string        `json:"owner"`
	Sender            string        `json:"sender"`
	OwnerDisplayName  string        `json:"ownerDisplayName"`
	SenderDisplayName string        `json:"senderDisplayName"`
	ShareType         string        `json:"shareType"`
	ResourceType      string        `json:"resourceType"`
	Protocol          *protocolInfo `json:"protocol"`

	ID        string `json:"id,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

func (s *share) JSON() []byte {
	b, _ := json.MarshalIndent(s, "", "   ")
	return b

}

type protocolInfo struct {
	Name    string           `json:"name"`
	Options *protocolOptions `json:"options"`
}

type protocolOptions struct {
	SharedSecret string `json:"sharedSecret,omitempty"`
	Permissions  string `json:"permissions,omitempty"`
}

type userManager interface {
	UserExists(ctx context.Context, username string) error
}

type providerAuthorizer interface {
	IsProviderAllowed(ctx context.Context, domain string) error
	GetProviderInfoByDomain(ctx context.Context, domain string) (*providerInfo, error)
	AddProvider(ctx context.Context, p *providerInfo) error
}

type providerInfo struct {
	Domain         string
	APIVersion     string
	APIEndPoint    string
	WebdavEndPoint string
}

type shareManager interface {
	GetInternalShare(ctx context.Context, id string) (*share, error)
	NewShare(ctx context.Context, share *share, domain, shareWith string) (*share, error)
	GetShares(ctx context.Context, user string) ([]*share, error)
	GetExternalShare(ctx context.Context, sharedWith, id string) (*share, error)
}

type tokenManager interface {
	IsValid(ctx context.Context, u *url.URL, token string) error
}

// HAL mambo-jambo for the format of the responses.
type halLinks struct {
	Self *halRef `json:"self"`
	Next *halRef `json:"next,omitempty"`
}

type halRef struct {
	Href string `json:"href"`
}

type halEmbedded struct {
	HALshares []*halSingleShareResponse `json:"shares"`
}

type halSingleShareResponse struct {
	*share
	*halLinks `json:"_links"`
}

func (ssr halSingleShareResponse) JSON() []byte {
	b, _ := json.MarshalIndent(ssr, "", "   ")
	return b
}

type halMultipleShareResponse struct {
	Embbeded *halEmbedded `json:"_embbeded"`
	Links    *halLinks    `json:"_links"`
}

func (msr halMultipleShareResponse) JSON() []byte {
	b, _ := json.MarshalIndent(msr, "", "   ")
	return b
}

type mySQLOptions struct {
	Hostname string
	Port     int
	Username string
	Password string
	DB       string
	Table    string

	Logger *zap.Logger
}

type apiInfo struct {
	Enabled       bool            `json:"enabled"`
	APIVersion    string          `json:"apiVersion"`
	EndPoint      string          `json:"endPoint"`
	ResourceTypes []resourceTypes `json:"resourceTypes"`
}

type resourceTypes struct {
	Name       string                 `json:"name"`
	ShareTypes []string               `json:"shareTypes"`
	Protocols  resourceTypesProtocols `json:"protocols"`
}

type resourceTypesProtocols struct {
	Webdav string `json:"webdav"`
}

func (s *apiInfo) JSON() []byte {
	b, _ := json.MarshalIndent(s, "", "   ")
	return b

}
