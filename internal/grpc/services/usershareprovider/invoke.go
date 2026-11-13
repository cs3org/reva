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

package usershareprovider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
)

const maxExpiringPage = 1000

type expiryLister interface {
	ExpiringShares(from, to, afterExpiration time.Time, afterID uint, limit int) ([]model.Share, error)
}

// Invocations implements invoke.Invokable.
func (s *service) Invocations() []invoke.InvocationSpec { return s.set.Invocations() }

// Invoke implements invoke.Invokable.
func (s *service) Invoke(ctx context.Context, name string, args map[string]any) (invoke.Result, error) {
	return s.set.Invoke(ctx, name, args)
}

type expiringArgs struct {
	From            time.Time `arg:"from,required" help:"start of the window"`
	To              time.Time `arg:"to,required" help:"end of the window, excluded"`
	AfterExpiration time.Time `arg:"after_expiration" help:"continue after the share with this expiration"`
	AfterID         int       `arg:"after_id" help:"continue after the share with this id"`
	Limit           int       `arg:"limit" help:"return at most this many shares, default and max 1000"`
}

func (s *service) registerInvocations() {
	s.set = invoke.NewSet()
	s.set.Add("expiring", "List the shares of all users that expire in a window, by expiration and id, orphans excluded.").
		Handle(invoke.Typed(s.expiring))
}

func (s *service) expiring(_ context.Context, a expiringArgs) (invoke.Result, error) {
	lister, ok := s.sm.(expiryLister)
	if !ok {
		return nil, fmt.Errorf("the %s driver cannot list shares by expiration", s.conf.Driver)
	}
	if !a.To.After(a.From) {
		return nil, errors.New("to must be after from")
	}
	limit := a.Limit
	if limit <= 0 || limit > maxExpiringPage {
		limit = maxExpiringPage
	}
	shares, err := lister.ExpiringShares(a.From, a.To, a.AfterExpiration, uint(a.AfterID), limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(shares))
	for _, sh := range shares {
		out = append(out, map[string]any{
			"id":         sh.Id,
			"owner":      sh.UIDOwner,
			"initiator":  sh.UIDInitiator,
			"instance":   sh.Instance,
			"inode":      sh.Inode,
			"item_type":  string(sh.ItemType),
			"share_with": sh.ShareWith,
			"group":      sh.SharedWithIsGroup,
			"expires_at": sh.Expiration.V.UTC().Format(time.RFC3339Nano),
			"updated_at": sh.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return invoke.Result{"shares": out}, nil
}
