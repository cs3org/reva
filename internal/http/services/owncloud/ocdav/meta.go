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
func (h *MetaHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		// 👈 ここから書き換える
		// エンコードが維持された生のパス（/remote.php/dav/meta/...）を取得
		rawPath := r.URL.RawPath
		if rawPath == "" {
			rawPath = r.URL.Path
		}

		// ルーターの手前までの不要なプレフィックス（/remote.php/dav/meta/）を削る
		// ※もしうまく動かない場合は、ここの削り方を調整します
		var id string
		id, rawPath = router.ShiftPath(rawPath) 

		// パーセントエンコード（%2Fなど）を本来の文字（/など）にデコードする
		decodedID, err := url.PathUnescape(id)
		if err != nil {
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}
		// 👈 ここまで書き換える

		// 以前の「id」を「decodedID」に差し替える
		rid, ok := spaces.ParseResourceID(decodedID)
		if !ok {
			var err error
			rid, err = spaces.ResourceIdFromString(decodedID)
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

		// 残りのパスを書き戻して次の処理へ回す
		r.URL.Path = rawPath
		r.URL.RawPath = rawPath

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
