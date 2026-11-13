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

package publicshareprovider

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
	ExpiringPublicLinks(from, to, afterExpiration time.Time, afterID uint, limit int) ([]model.PublicLink, error)
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
	AfterExpiration time.Time `arg:"after_expiration" help:"continue after the link with this expiration"`
	AfterID         int       `arg:"after_id" help:"continue after the link with this id"`
	Limit           int       `arg:"limit" help:"return at most this many links, default and max 1000"`
}

func (s *service) registerInvocations() {
	s.set = invoke.NewSet()
	s.set.Add("expiring", "List the public links of all users that expire in a window, by expiration and id, orphans excluded.").
		Handle(invoke.Typed(s.expiring))
}

func (s *service) expiring(_ context.Context, a expiringArgs) (invoke.Result, error) {
	lister, ok := s.sm.(expiryLister)
	if !ok {
		return nil, fmt.Errorf("the %s driver cannot list links by expiration", s.conf.Driver)
	}
	if !a.To.After(a.From) {
		return nil, errors.New("to must be after from")
	}
	limit := a.Limit
	if limit <= 0 || limit > maxExpiringPage {
		limit = maxExpiringPage
	}
	links, err := lister.ExpiringPublicLinks(a.From, a.To, a.AfterExpiration, uint(a.AfterID), limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(links))
	for _, l := range links {
		out = append(out, map[string]any{
			"id":         l.Id,
			"token":      l.Token,
			"owner":      l.UIDOwner,
			"initiator":  l.UIDInitiator,
			"instance":   l.Instance,
			"inode":      l.Inode,
			"item_type":  string(l.ItemType),
			"name":       l.LinkName,
			"expires_at": l.Expiration.V.UTC().Format(time.RFC3339Nano),
			"updated_at": l.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return invoke.Result{"links": out}, nil
}
