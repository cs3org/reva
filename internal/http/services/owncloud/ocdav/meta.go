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

package ocdav

import (
	"net/http"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

// MetaHandler handles meta requests.
type MetaHandler struct {
	VersionsHandler *VersionsHandler
}

func (h *MetaHandler) init(c *Config) error {
	h.VersionsHandler = new(VersionsHandler)
	return h.VersionsHandler.init(c)
}

// Handler handles requests.
// Handler handles requests.
func (h *MetaHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		// --- 🛠️ ここから追加・修正 ---
		// RawPath（エンコード維持）が空なら Path を使う
		p := r.URL.RawPath
		if p == "" {
			p = r.URL.Path
		}

		var id string
		// 共通の変数 p を使ってパスを切り出す
		id, p = router.ShiftPath(p)
		if id == "" {
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}

		// 切り出した後の残りのパスを、URLオブジェクトのPathとRawPathにそれぞれ同期させる
		// （※idの部分はURLデコードしてあげる必要があります）
		r.URL.Path = r.URL.Path[len(id)+1:] // 簡易的な同期（文字数がズレる場合は要調整）
		r.URL.RawPath = p
		// --- 🛠️ ここまで ---

		rid, ok := spaces.ParseResourceID(id)
		if !ok {
			// If this fails, client might be non-spaces
			var err error
			rid, err = spaces.ResourceIdFromString(id)
			if err != nil {
				http.Error(w, "400 Bad Request", http.StatusBadRequest)
				return
			}
		}

		log.Debug().
			Str("storage_id", rid.StorageId).
			Str("space_id", rid.SpaceId).
			Str("opaque_id", rid.OpaqueId).
			Msg("meta: parsed resource ID")

		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
		switch head {
		case "v":
			h.VersionsHandler.Handler(s, rid).ServeHTTP(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
