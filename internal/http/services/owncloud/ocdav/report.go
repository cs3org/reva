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
	"strings"

	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
)

const (
	elementNameSearchFiles = "search-files"
	elementNameFilterFiles = "filter-files"
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

	if rep.FilterFiles != nil {
		s.doFilterFiles(w, r, rep.FilterFiles, ns)
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

func (s *svc) doFilterFiles(w http.ResponseWriter, r *http.Request, ff *reportFilterFiles, namespace string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	if ff.Rules.Favorite {
		// List the users favorite resources.
		currentUser := ctxpkg.ContextMustGetUser(ctx)
		favorites, err := s.favoritesManager.ListFavorites(ctx, currentUser.Id)
		if err != nil {
			log.Error().Err(err).Msg("error getting favorites")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		client, err := s.getClient()
		if err != nil {
			log.Error().Err(err).Msg("error getting gateway client")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		infos := make([]*provider.ResourceInfo, 0, len(favorites))
		for i := range favorites {
			statRes, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: favorites[i]}})
			if err != nil {
				log.Error().Err(err).Msg("error getting resource info")
				continue
			}
			if statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
				log.Error().Interface("stat_response", statRes).Msg("error getting resource info")
				continue
			}

			// If global URLs are not supported, return only the file path
			if s.c.WebdavNamespace != "" {
				// The paths we receive have the format /user/<username>/<filepath>
				// We only want the `<filepath>` part. Thus we remove the /user/<username>/ part.
				parts := strings.SplitN(statRes.Info.Path, "/", 4)
				if len(parts) != 4 {
					log.Error().Str("path", statRes.Info.Path).Msg("path doesn't have the expected format")
					continue
				}
				statRes.Info.Path = parts[3]
			}

			infos = append(infos, statRes.Info)
		}

		responsesXML, err := s.multistatusResponse(ctx, &propfindXML{Prop: ff.Prop}, infos, namespace, nil, nil)
		if err != nil {
			log.Error().Err(err).Msg("error formatting propfind")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set(HeaderDav, "1, 3, extended-mkcol")
		w.Header().Set(HeaderContentType, "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		if _, err := w.Write([]byte(responsesXML)); err != nil {
			log.Err(err).Msg("error writing response")
		}
	}
}

type report struct {
	SearchFiles *reportSearchFiles
	// FilterFiles TODO add this for tag based search
	FilterFiles *reportFilterFiles `xml:"filter-files"`
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

type reportFilterFiles struct {
	XMLName xml.Name               `xml:"filter-files"`
	Lang    string                 `xml:"xml:lang,attr,omitempty"`
	Prop    propfindProps          `xml:"DAV: prop"`
	Rules   reportFilterFilesRules `xml:"filter-rules"`
}

type reportFilterFilesRules struct {
	Favorite  bool `xml:"favorite"`
	SystemTag int  `xml:"systemtag"`
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
			if v.Name.Local == elementNameSearchFiles {
				var repSF reportSearchFiles
				err = decoder.DecodeElement(&repSF, &v)
				if err != nil {
					return nil, http.StatusBadRequest, err
				}
				rep.SearchFiles = &repSF
			} else if v.Name.Local == elementNameFilterFiles {
				var repFF reportFilterFiles
				err = decoder.DecodeElement(&repFF, &v)
				if err != nil {
					return nil, http.StatusBadRequest, err
				}
				rep.FilterFiles = &repFF
			}
		}
	}
}
