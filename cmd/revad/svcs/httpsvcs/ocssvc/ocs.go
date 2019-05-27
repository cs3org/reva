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
	"encoding/json"
	"encoding/xml"
	"net/http"
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

// WriteOCSResponse handles writing ocs responses in json and xml
func WriteOCSResponse(w http.ResponseWriter, r *http.Request, res *Response) error {
	var encoded []byte
	var err error
	if r.URL.Query().Get("format") == "xml" {
		w.Header().Set("Content-Type", "application/xml")
		_, err = w.Write([]byte(xml.Header))
		if err != nil {
			return err
		}
		encoded, err = xml.Marshal(res.OCS)
	} else {
		w.Header().Set("Content-Type", "application/json")
		encoded, err = json.Marshal(res)
	}
	if err != nil {
		return err
	}

	_, err = w.Write(encoded)
	if err != nil {
		return err
	}
	return nil
}
