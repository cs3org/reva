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

package dataprovider

import (
	"net/http"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
)

func (s *svc) doPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	fn := r.URL.Path
	defer r.Body.Close()

	fsfn := strings.TrimPrefix(fn, s.conf.Prefix)
	ref := &provider.Reference{Spec: &provider.Reference_Path{Path: fsfn}}

	err := s.storage.Upload(ctx, ref, r.Body)
	if err != nil {
		if _, ok := err.(errtypes.IsPartialContent); ok {
			w.WriteHeader(http.StatusPartialContent)
			return
		}
		log.Error().Err(err).Msg("error uploading file")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
