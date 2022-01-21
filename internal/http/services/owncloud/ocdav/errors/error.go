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

package errors

import (
	"bytes"
	"encoding/xml"
	"net/http"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

var sabreException = map[int]string{

	//http.StatusMultipleChoices:   "Multiple Choices",
	//http.StatusMovedPermanently:  "Moved Permanently",
	//http.StatusFound:             "Found",
	//http.StatusSeeOther:          "See Other",
	//http.StatusNotModified:       "Not Modified",
	//http.StatusUseProxy:          "Use Proxy",
	//http.StatusTemporaryRedirect: "Temporary Redirect",
	//http.StatusPermanentRedirect: "Permanent Redirect",

	http.StatusBadRequest:       "Sabre\\DAV\\Exception\\BadRequest",
	http.StatusUnauthorized:     "Sabre\\DAV\\Exception\\NotAuthenticated",
	http.StatusPaymentRequired:  "Sabre\\DAV\\Exception\\PaymentRequired",
	http.StatusForbidden:        "Sabre\\DAV\\Exception\\Forbidden", // InvalidResourceType, InvalidSyncToken, TooManyMatches
	http.StatusNotFound:         "Sabre\\DAV\\Exception\\NotFound",
	http.StatusMethodNotAllowed: "Sabre\\DAV\\Exception\\MethodNotAllowed",
	//http.StatusNotAcceptable:                "Not Acceptable",
	//http.StatusProxyAuthRequired:            "Proxy Authentication Required",
	//http.StatusRequestTimeout:               "Request Timeout",
	http.StatusConflict: "Sabre\\DAV\\Exception\\Conflict", // LockTokenMatchesRequestUri
	//http.StatusGone:                         "Gone",
	http.StatusLengthRequired:     "Sabre\\DAV\\Exception\\LengthRequired",
	http.StatusPreconditionFailed: "Sabre\\DAV\\Exception\\PreconditionFailed",
	//http.StatusRequestEntityTooLarge:        "Request Entity Too Large",
	//http.StatusRequestURITooLong:            "Request URI Too Long",
	http.StatusUnsupportedMediaType:         "Sabre\\DAV\\Exception\\UnsupportedMediaType", // ReportNotSupported
	http.StatusRequestedRangeNotSatisfiable: "Sabre\\DAV\\Exception\\RequestedRangeNotSatisfiable",
	//http.StatusExpectationFailed:            "Expectation Failed",
	//http.StatusTeapot:                       "I'm a teapot",
	//http.StatusMisdirectedRequest:           "Misdirected Request",
	//http.StatusUnprocessableEntity:          "Unprocessable Entity",
	http.StatusLocked: "Sabre\\DAV\\Exception\\Locked", // ConflictingLock
	//http.StatusFailedDependency:             "Failed Dependency",
	//http.StatusTooEarly:                     "Too Early",
	//http.StatusUpgradeRequired:              "Upgrade Required",
	//http.StatusPreconditionRequired:         "Precondition Required",
	//http.StatusTooManyRequests:              "Too Many Requests",
	//http.StatusRequestHeaderFieldsTooLarge:  "Request Header Fields Too Large",
	//http.StatusUnavailableForLegalReasons:   "Unavailable For Legal Reasons",

	//http.StatusInternalServerError:           "Internal Server Error",
	http.StatusNotImplemented: "Sabre\\DAV\\Exception\\NotImplemented",
	//http.StatusBadGateway:                    "Bad Gateway",
	http.StatusServiceUnavailable: "Sabre\\DAV\\Exception\\ServiceUnavailable",
	//http.StatusGatewayTimeout:                "Gateway Timeout",
	//http.StatusHTTPVersionNotSupported:       "HTTP Version Not Supported",
	//http.StatusVariantAlsoNegotiates:         "Variant Also Negotiates",
	http.StatusInsufficientStorage: "Sabre\\DAV\\Exception\\InsufficientStorage",
	//http.StatusLoopDetected:                  "Loop Detected",
	//http.StatusNotExtended:                   "Not Extended",
	//http.StatusNetworkAuthenticationRequired: "Network Authentication Required",
}

// SabreException returns a sabre exception text for the HTTP status code. It returns the empty
// string if the code is unknown.
func SabreException(code int) string {
	return sabreException[code]
}

// Exception represents a ocdav exception
type Exception struct {
	Code    int
	Message string
	Header  string
}

// Marshal just calls the xml marshaller for a given exception.
func Marshal(code int, message string, header string) ([]byte, error) {
	xmlstring, err := xml.Marshal(&ErrorXML{
		Xmlnsd:    "DAV",
		Xmlnss:    "http://sabredav.org/ns",
		Exception: sabreException[code],
		Message:   message,
		Header:    header,
	})
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.Write(xmlstring)
	return buf.Bytes(), err
}

// ErrorXML holds the xml representation of an error
// http://www.webdav.org/specs/rfc4918.html#ELEMENT_error
type ErrorXML struct {
	XMLName   xml.Name `xml:"d:error"`
	Xmlnsd    string   `xml:"xmlns:d,attr"`
	Xmlnss    string   `xml:"xmlns:s,attr"`
	Exception string   `xml:"s:exception"`
	Message   string   `xml:"s:message"`
	InnerXML  []byte   `xml:",innerxml"`
	// Header is used to indicate the conflicting request header
	Header string `xml:"s:header,omitempty"`
}

var (
	// ErrInvalidDepth is an invalid depth header error
	ErrInvalidDepth = errors.New("webdav: invalid depth")
	// ErrInvalidPropfind is an invalid propfind error
	ErrInvalidPropfind = errors.New("webdav: invalid propfind")
	// ErrInvalidProppatch is an invalid proppatch error
	ErrInvalidProppatch = errors.New("webdav: invalid proppatch")
	// ErrUnsupportedLockInfo is an invalid lock error
	ErrInvalidLockInfo = errors.New("webdav: invalid lock info")
	// ErrUnsupportedLockInfo is an unsupported lock error
	ErrUnsupportedLockInfo = errors.New("webdav: unsupported lock info")
	// ErrInvalidTimeout is an invalid timeout error
	ErrInvalidTimeout = errors.New("webdav: invalid timeout")
	// ErrInvalidIfHeader is an invalid if header error
	ErrInvalidIfHeader = errors.New("webdav: invalid If header")
	// ErrUnsupportedMethod is an unsupported method error
	ErrUnsupportedMethod = errors.New("webdav: unsupported method")
	// ErrInvalidLockToken is an invalid lock token error
	ErrInvalidLockToken = errors.New("webdav: invalid lock token")
	// ErrConfirmationFailed is returned by a LockSystem's Confirm method.
	ErrConfirmationFailed = errors.New("webdav: confirmation failed")
	// ErrForbidden is returned by a LockSystem's Unlock method.
	ErrForbidden = errors.New("webdav: forbidden")
	// ErrLocked is returned by a LockSystem's Create, Refresh and Unlock methods.
	ErrLocked = errors.New("webdav: locked")
	// ErrNoSuchLock is returned by a LockSystem's Refresh and Unlock methods.
	ErrNoSuchLock = errors.New("webdav: no such lock")
	// ErrNotImplemented is returned when hitting not implemented code paths
	ErrNotImplemented = errors.New("webdav: not implemented")
)

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
