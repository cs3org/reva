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

package ocs

import (
	"encoding/json"
	"encoding/xml"
	"net/http"

	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	"github.com/cs3org/reva/pkg/appctx"
)

// Response is the top level response structure
type Response struct {
	OCS *Payload `json:"ocs"`
}

// Payload combines response metadata and data
type Payload struct {
	XMLName struct{}      `json:"-" xml:"ocs"`
	Meta    *ResponseMeta `json:"meta" xml:"meta"`
	Data    interface{}   `json:"data,omitempty" xml:"data,omitempty"`
}

// ResponseMeta holds response metadata
type ResponseMeta struct {
	Status       string `json:"status" xml:"status"`
	StatusCode   int    `json:"statuscode" xml:"statuscode"`
	Message      string `json:"message" xml:"message"`
	TotalItems   string `json:"totalitems,omitempty" xml:"totalitems,omitempty"`
	ItemsPerPage string `json:"itemsperpage,omitempty" xml:"itemsperpage,omitempty"`
}

// MetaOK is the default ok response
var MetaOK = &ResponseMeta{Status: "ok", StatusCode: 100, Message: "OK"}

// MetaBadRequest is used for unknown errers
var MetaBadRequest = &ResponseMeta{Status: "error", StatusCode: 400, Message: "Bad Request"}

// MetaServerError is returned on server errors
var MetaServerError = &ResponseMeta{Status: "error", StatusCode: 996, Message: "Server Error"}

// MetaUnauthorized is returned on unauthorized requests
var MetaUnauthorized = &ResponseMeta{Status: "error", StatusCode: 997, Message: "Unauthorised"}

// MetaNotFound is returned when trying to access not existing resources
var MetaNotFound = &ResponseMeta{Status: "error", StatusCode: 998, Message: "Not Found"}

// MetaUnknownError is used for unknown errers
var MetaUnknownError = &ResponseMeta{Status: "error", StatusCode: 999, Message: "Unknown Error"}

// WriteOCSSuccess handles writing successful ocs response data
func WriteOCSSuccess(w http.ResponseWriter, r *http.Request, d interface{}) {
	WriteOCSData(w, r, MetaOK, d, nil)
}

// WriteOCSError handles writing error ocs responses
func WriteOCSError(w http.ResponseWriter, r *http.Request, c int, m string, err error) {
	WriteOCSData(w, r, &ResponseMeta{Status: "error", StatusCode: c, Message: m}, nil, err)
}

// WriteOCSData handles writing ocs data in json and xml
func WriteOCSData(w http.ResponseWriter, r *http.Request, m *ResponseMeta, d interface{}, err error) {
	WriteOCSResponse(w, r, &Response{
		OCS: &Payload{
			Meta: m,
			Data: d,
		},
	}, err)
}

// WriteOCSResponse handles writing ocs responses in json and xml
func WriteOCSResponse(w http.ResponseWriter, r *http.Request, res *Response, err error) {
	var encoded []byte

	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg(res.OCS.Meta.Message)
	}

	if r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json")
		encoded, err = json.Marshal(res)
	} else {
		w.Header().Set("Content-Type", "application/xml")
		_, err = w.Write([]byte(xml.Header))
		if err != nil {
			appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing xml header")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		encoded, err = xml.Marshal(res.OCS)
	}
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("error encoding ocs response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO map error for v2 only?
	// see https://github.com/owncloud/core/commit/bacf1603ffd53b7a5f73854d1d0ceb4ae545ce9f#diff-262cbf0df26b45bad0cf00d947345d9c
	switch res.OCS.Meta.StatusCode {
	case MetaNotFound.StatusCode:
		w.WriteHeader(http.StatusNotFound)
	case MetaServerError.StatusCode:
		w.WriteHeader(http.StatusInternalServerError)
	case MetaUnknownError.StatusCode:
		w.WriteHeader(http.StatusInternalServerError)
	case MetaUnauthorized.StatusCode:
		w.WriteHeader(http.StatusUnauthorized)
	case 100:
		w.WriteHeader(http.StatusOK)
	case 104:
		w.WriteHeader(http.StatusForbidden)
	default:
		// any 2xx, 4xx and 5xx will be used as is
		if res.OCS.Meta.StatusCode >= 200 && res.OCS.Meta.StatusCode < 600 {
			w.WriteHeader(res.OCS.Meta.StatusCode)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}

	_, err = w.Write(encoded)
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing ocs response")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// UserIDToString returns a userid string with an optional idp separated by @: "<opaque id>[@<idp>]"
func UserIDToString(userID *typespb.UserId) string {
	if userID == nil || userID.OpaqueId == "" {
		return ""
	}
	if userID.Idp == "" {
		return userID.OpaqueId
	}
	return userID.OpaqueId + "@" + userID.Idp
}
