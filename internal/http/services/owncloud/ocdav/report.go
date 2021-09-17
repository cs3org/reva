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
	"io"
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
)

func (s *svc) handleReport(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	// fn := path.Join(ns, r.URL.Path)

	rep, status, err := readReport(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("error reading report")
		w.WriteHeader(status)
		return
	}
	if rep.SearchFiles != nil {
		s.doSearchFiles(w, r, rep.SearchFiles)
		return
	}

	// TODO(jfd): implement report

	w.WriteHeader(http.StatusNotImplemented)
}

func (s *svc) doSearchFiles(w http.ResponseWriter, r *http.Request, sf *reportSearchFiles) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	_, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNotImplemented)
}

type report struct {
	SearchFiles *reportSearchFiles
	// FilterFiles TODO add this for tag based search
}
type reportSearchFiles struct {
	XMLName xml.Name                `xml:"search-files"`
	Lang    string                  `xml:"xml:lang,attr,omitempty"`
	Prop    propfindProps           `xml:"DAV: prop"`
	Search  reportSearchFilesSearch `xml:"search"`
}
type reportSearchFilesSearch struct {
	Pattern string `xml:"search"`
	Limit   int    `xml:"limit"`
	Offset  int    `xml:"offset"`
}

func readReport(r io.Reader) (rep *report, status int, err error) {
	decoder := xml.NewDecoder(r)
	rep = &report{}
	for {
		t, err := decoder.Token()
		if err == io.EOF {
			// io.EOF is a successful end
			return rep, 0, nil
		}
		if err != nil {
			return nil, http.StatusBadRequest, err
		}

		if v, ok := t.(xml.StartElement); ok {
			if v.Name.Local == "search-files" {
				var repSF reportSearchFiles
				err = decoder.DecodeElement(&repSF, &v)
				if err != nil {
					return nil, http.StatusBadRequest, err
				}
				rep.SearchFiles = &repSF
			}
		}
	}
}
