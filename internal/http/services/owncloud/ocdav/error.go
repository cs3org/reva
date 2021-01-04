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

package ocdav

import (
	"encoding/xml"
	"net/http"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/rs/zerolog"
)

type code int

const (
	// SabredavMethodNotAllowed maps to HTTP 405
	SabredavMethodNotAllowed code = iota
)

var (
	codesEnum = []string{
		"Sabre\\DAV\\Exception\\MethodNotAllowed",
	}
)

type exception struct {
	code    code
	message string
}

// Marshal just calls the xml marshaller for a given exception.
func Marshal(e exception) ([]byte, error) {
	return xml.Marshal(&errorXML{
		Xmlnsd:    "DAV",
		Xmlnss:    "http://sabredav.org/ns",
		Exception: codesEnum[e.code],
		Message:   e.message,
	})
}

func handleErrorStatus(log *zerolog.Logger, w http.ResponseWriter, s *rpc.Status) {
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
	case rpc.Code_CODE_INVALID_ARGUMENT:
		log.Debug().Interface("status", s).Msg("bad request")
		w.WriteHeader(http.StatusBadRequest)
	case rpc.Code_CODE_UNIMPLEMENTED:
		log.Debug().Interface("status", s).Msg("not implemented")
		w.WriteHeader(http.StatusNotImplemented)
	default:
		log.Error().Interface("status", s).Msg("grpc request failed")
		w.WriteHeader(http.StatusInternalServerError)
	}
}
