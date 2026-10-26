// Copyright 2018-2026 CERN
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

package userprovider

import (
	"context"
	"slices"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/utils"
)

type userArgs struct {
	User  string `arg:"user" help:"the username"`
	Claim string `arg:"claim" help:"find the user by this claim instead of the username, e.g. mail or uid"`
	Value string `arg:"value" help:"the value of the claim"`
}

type findArgs struct {
	Query string `arg:"query,required" help:"text to find in the username, display name or mail"`
}

// The invocations are Shared because all the instances read the same
// directory, so one answer is sufficient.
func (s *service) registerInvocations() {
	s.Add("get", "Show a user: id, type, status, mail and uid/gid numbers.").
		Shared().
		Handle(invoke.Typed(s.get))
	s.Add("groups", "List the groups of a user, as the driver resolves them.").
		Shared().
		Handle(invoke.Typed(s.groups))
	s.Add("find", "Find the users of all types that match a query, sorted by username.").
		Shared().
		Handle(invoke.Typed(s.find))
}

func (s *service) lookup(ctx context.Context, a userArgs) (*userpb.User, error) {
	claim, value := "username", a.User
	if a.Claim != "" {
		claim, value = a.Claim, a.Value
	}
	if (a.User == "") == (a.Claim == "") || value == "" {
		return nil, errtypes.BadRequest("give user=, or claim= and value=")
	}
	// The groups invocation gives the groups, and some drivers need more calls to fetch them.
	return s.usermgr.GetUserByClaim(ctx, claim, value, true)
}

func (s *service) get(ctx context.Context, a userArgs) (invoke.Result, error) {
	u, err := s.lookup(ctx, a)
	if err != nil {
		return nil, err
	}
	return userResult(u), nil
}

func (s *service) groups(ctx context.Context, a userArgs) (invoke.Result, error) {
	u, err := s.lookup(ctx, a)
	if err != nil {
		return nil, err
	}
	groups, err := s.usermgr.GetUserGroups(ctx, u.GetId())
	if err != nil {
		return nil, err
	}
	slices.Sort(groups)
	return invoke.Result{"user": u.GetUsername(), "groups": groups}, nil
}

func (s *service) find(ctx context.Context, a findArgs) (invoke.Result, error) {
	// Without a filter, a driver can find only the primary accounts.
	filters := []*userpb.Filter{{Type: userpb.Filter_TYPE_QUERY, Term: &userpb.Filter_Query{Query: a.Query}}}
	users, err := s.usermgr.FindUsers(ctx, a.Query, filters, true)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(users, func(x, y *userpb.User) int { return strings.Compare(x.GetUsername(), y.GetUsername()) })
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, userResult(u))
	}
	return invoke.Result{"users": out}, nil
}

func userResult(u *userpb.User) map[string]any {
	return map[string]any{
		"id":            u.GetId().GetOpaqueId(),
		"idp":           u.GetId().GetIdp(),
		"type":          utils.UserTypeToString(u.GetId().GetType()),
		"username":      u.GetUsername(),
		"display_name":  u.GetDisplayName(),
		"mail":          u.GetMail(),
		"mail_verified": u.GetMailVerified(),
		"uid_number":    u.GetUidNumber(),
		"gid_number":    u.GetGidNumber(),
		"status":        strings.ToLower(strings.TrimPrefix(u.GetStatus().String(), "USER_STATUS_")),
	}
}
