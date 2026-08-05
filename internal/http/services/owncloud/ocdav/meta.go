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
	"strings" // 👈 パス文字列の分解のために追加

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
func (h *MetaHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		// 🛠️ ルーターが壊す前の「生のURLパス」を取得する
		// 例: /remote.php/dav/meta/localfs%24F5WG...%21fileid-%2Ftest.txt/v
		fullPath := r.URL.RawPath
		if fullPath == "" {
			fullPath = r.URL.Path
		}

		// ベースとなるパス（/remote.php/dav/meta/）より後ろの部分を抜き出す
		// これにより、%2Fが維持されたままのIDを取得できます
		basePath := "/remote.php/dav/meta/"
		subPath := strings.TrimPrefix(fullPath, basePath)

		// 最初の一塊（ID部分）と、それ以降（/v など）に分割する
		parts := strings.SplitN(subPath, "/", 2)
		id := parts[0]

		if id == "" {
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}

		// 残りのパス（"v" など）をルーター用に再設定する
		remaining := ""
		if len(parts) > 1 {
			remaining = parts[1]
		}
		r.URL.Path = "/" + remaining
		r.URL.RawPath = "/" + remaining

		// 以降は元のRevaの処理のまま、idを使って解析します
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
