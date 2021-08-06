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

package ocdav

import (
	"encoding/xml"
	"net/http"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type code int

const (

	// SabredavBadRequest maps to HTTP 400
	SabredavBadRequest code = iota
	// SabredavMethodNotAllowed maps to HTTP 405
	SabredavMethodNotAllowed
	// SabredavNotAuthenticated maps to HTTP 401
	SabredavNotAuthenticated
	// SabredavPreconditionFailed maps to HTTP 412
	SabredavPreconditionFailed
	// SabredavPermissionDenied maps to HTTP 403
	SabredavPermissionDenied
	// SabredavNotFound maps to HTTP 404
	SabredavNotFound
	// SabredavConflict maps to HTTP 409
	SabredavConflict
)

var (
	codesEnum = []string{
		"Sabre\\DAV\\Exception\\BadRequest",
		"Sabre\\DAV\\Exception\\MethodNotAllowed",
		"Sabre\\DAV\\Exception\\NotAuthenticated",
		"Sabre\\DAV\\Exception\\PreconditionFailed",
		"Sabre\\DAV\\Exception\\PermissionDenied",
		"Sabre\\DAV\\Exception\\NotFound",
		"Sabre\\DAV\\Exception\\Conflict",
	}
)

type exception struct {
	code    code
	message string
	header  string
}

// Marshal just calls the xml marshaller for a given exception.
func Marshal(e exception) ([]byte, error) {
	xmlstring, err := xml.Marshal(&errorXML{
		Xmlnsd:    "DAV",
		Xmlnss:    "http://sabredav.org/ns",
		Exception: codesEnum[e.code],
		Message:   e.message,
		Header:    e.header,
	})
	if err != nil {
		return []byte(""), err
	}
	return []byte(xml.Header + string(xmlstring)), err
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_error
type errorXML struct {
	XMLName   xml.Name `xml:"d:error"`
	Xmlnsd    string   `xml:"xmlns:d,attr"`
	Xmlnss    string   `xml:"xmlns:s,attr"`
	Exception string   `xml:"s:exception"`
	Message   string   `xml:"s:message"`
	InnerXML  []byte   `xml:",innerxml"`
	// Header is used to indicate the conflicting request header
	Header string `xml:"s:header,omitempty"`
}

var errInvalidPropfind = errors.New("webdav: invalid propfind")

// HandleErrorStatus checks the status code, logs a Debug or Error level message
// and writes an appropriate http status
func HandleErrorStatus(log *zerolog.Logger, w http.ResponseWriter, s *rpc.Status) {
	switch s.Code {
	case rpc.Code_CODE_OK:
		log.Debug().Interface("status", s).Msg("ok")
		w.WriteHeader(http.StatusOK)
	case rpc.Code_CODE_NOT_FOUND:
		log.Debug().Interface("status", s).Msg("resource not found")
		w.WriteHeader(http.StatusNotFound)
	case rpc.Code_CODE_PERMISSION_DENIED:
		log.Debug().Interface("status", s).Msg("permission denied")
		w.WriteHeader(http.StatusForbidden)
	case rpc.Code_CODE_UNAUTHENTICATED:
		log.Debug().Interface("status", s).Msg("unauthenticated")
		w.WriteHeader(http.StatusUnauthorized)
	case rpc.Code_CODE_INVALID_ARGUMENT:
		log.Debug().Interface("status", s).Msg("bad request")
		w.WriteHeader(http.StatusBadRequest)
	case rpc.Code_CODE_UNIMPLEMENTED:
		log.Debug().Interface("status", s).Msg("not implemented")
		w.WriteHeader(http.StatusNotImplemented)
	case rpc.Code_CODE_INSUFFICIENT_STORAGE:
		log.Debug().Interface("status", s).Msg("insufficient storage")
		w.WriteHeader(http.StatusInsufficientStorage)
	case rpc.Code_CODE_FAILED_PRECONDITION:
		log.Debug().Interface("status", s).Msg("destination does not exist")
		w.WriteHeader(http.StatusConflict)
	default:
		log.Error().Interface("status", s).Msg("grpc request failed")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// HandleWebdavError checks the status code, logs an error and creates a webdav response body
// if needed
func HandleWebdavError(log *zerolog.Logger, w http.ResponseWriter, b []byte, err error) {
	if err != nil {
		log.Error().Msgf("error marshaling xml response: %s", b)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, err = w.Write(b)
	if err != nil {
		log.Err(err).Msg("error writing response")
	}
}
