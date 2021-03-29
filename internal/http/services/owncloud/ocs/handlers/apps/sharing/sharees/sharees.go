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

package sharees

import (
	"net/http"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
)

// Handler implements the ownCloud sharing API
type Handler struct {
	gatewayAddr             string
	additionalInfoAttribute string
}

// Init initializes this and any contained handlers
func (h *Handler) Init(c *config.Config) error {
	h.gatewayAddr = c.GatewaySvc
	h.additionalInfoAttribute = c.AdditionalInfoAttribute
	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	var head string
	head, r.URL.Path = router.ShiftPath(r.URL.Path)

	log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")

	h.findSharees(w, r)
}

func (h *Handler) findSharees(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())
	term := r.URL.Query().Get("search")

	if term == "" {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "search must not be empty", nil)
		return
	}

	gwc, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting gateway grpc client", err)
		return
	}

	usersRes, err := gwc.FindUsers(r.Context(), &userpb.FindUsersRequest{Filter: term})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error searching users", err)
		return
	}
	log.Debug().Int("count", len(usersRes.GetUsers())).Str("search", term).Msg("users found")

	userMatches := make([]*conversions.MatchData, 0, len(usersRes.GetUsers()))
	for _, user := range usersRes.GetUsers() {
		match := h.userAsMatch(user)
		log.Debug().Interface("user", user).Interface("match", match).Msg("mapped")
		userMatches = append(userMatches, match)
	}

	groupsRes, err := gwc.FindGroups(r.Context(), &grouppb.FindGroupsRequest{Filter: term})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error searching groups", err)
		return
	}
	log.Debug().Int("count", len(groupsRes.GetGroups())).Str("search", term).Msg("groups found")

	groupMatches := make([]*conversions.MatchData, 0, len(groupsRes.GetGroups()))
	for _, g := range groupsRes.GetGroups() {
		match := h.groupAsMatch(g)
		log.Debug().Interface("group", g).Interface("match", match).Msg("mapped")
		groupMatches = append(groupMatches, match)
	}

	response.WriteOCSSuccess(w, r, &conversions.ShareeData{
		Exact: &conversions.ExactMatchesData{
			Users:   []*conversions.MatchData{},
			Groups:  []*conversions.MatchData{},
			Remotes: []*conversions.MatchData{},
		},
		Users:   userMatches,
		Groups:  groupMatches,
		Remotes: []*conversions.MatchData{},
	})
}

func (h *Handler) userAsMatch(u *userpb.User) *conversions.MatchData {
	return &conversions.MatchData{
		Label: u.DisplayName,
		Value: &conversions.MatchValueData{
			ShareType: int(conversions.ShareTypeUser),
			// api compatibility with oc10: always use the username
			ShareWith:               u.Username,
			ShareWithAdditionalInfo: h.getAdditionalInfoAttribute(u),
		},
	}
}

func (h *Handler) groupAsMatch(g *grouppb.Group) *conversions.MatchData {
	return &conversions.MatchData{
		Label: g.DisplayName,
		Value: &conversions.MatchValueData{
			ShareType:               int(conversions.ShareTypeGroup),
			ShareWith:               g.GroupName,
			ShareWithAdditionalInfo: g.Mail,
		},
	}
}

func (h *Handler) getAdditionalInfoAttribute(u *userpb.User) string {
	return templates.WithUser(u, h.additionalInfoAttribute)
}
