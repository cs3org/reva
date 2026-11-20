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

// Package shareexpiry tells users that shares expire soon: the sharer, so
// that they can extend the share, and the grantee, so that they know when
// their access ends.
package shareexpiry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/reminders"
	"github.com/cs3org/reva/v3/pkg/reminders/registry"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

const pageSize = 500

const (
	notifySharer  = "sharer"
	notifyGrantee = "grantee"
)

func init() {
	registry.Register("share_expiry", New)
}

type config struct {
	// DaysBefore are the reminder points: a share is reminded when it has
	// that many days left or less. The level of a share is the rank of the
	// nearest point that it reached, the furthest point being 1.
	DaysBefore []int  `mapstructure:"days_before"`
	BaseURL    string `mapstructure:"base_url"`
	// Notify selects who is told: "sharer", "grantee" or both.
	Notify []string `mapstructure:"notify"`
	// RecipientRole is the role of a project space whose users and groups
	// are reminded, as sharers, of the shares in the project.
	RecipientRole    string `mapstructure:"recipient_role"`
	MaxSharesPerMail int    `mapstructure:"max_shares_per_mail"`
}

func (c *config) ApplyDefaults() {
	if len(c.DaysBefore) == 0 {
		c.DaysBefore = []int{7, 1}
	}
	slices.Sort(c.DaysBefore)
	slices.Reverse(c.DaysBefore)
	c.DaysBefore = slices.Compact(c.DaysBefore)
	if c.BaseURL == "" {
		c.BaseURL = "https://cernbox.cern.ch"
	}
	c.BaseURL = strings.TrimSuffix(c.BaseURL, "/")
	if len(c.Notify) == 0 {
		c.Notify = []string{notifySharer}
	}
	if c.RecipientRole == "" {
		c.RecipientRole = "admin"
	}
	if c.MaxSharesPerMail <= 0 {
		c.MaxSharesPerMail = 20
	}
}

type kind struct {
	conf config
	// When nil, the clients come from the service registry.
	admin   adminpb.AdminAPIClient
	gateway gateway.GatewayAPIClient
	now     func() time.Time
}

// New builds the kind from its configuration.
func New(_ context.Context, m map[string]any) (reminders.Reminder, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	for _, n := range c.Notify {
		if n != notifySharer && n != notifyGrantee {
			return nil, fmt.Errorf("share_expiry: unknown notify %q, want %q or %q", n, notifySharer, notifyGrantee)
		}
	}
	return &kind{conf: c, now: time.Now}, nil
}

func (k *kind) Event() string { return model.EventShareExpiry }

// share is a share as the expiring invocation of the user share provider
// returns it.
type share struct {
	ID        uint      `json:"id"`
	Initiator string    `json:"initiator"`
	Instance  string    `json:"instance"`
	Inode     string    `json:"inode"`
	ItemType  string    `json:"item_type"`
	ShareWith string    `json:"share_with"`
	Group     bool      `json:"group"`
	ExpiresAt time.Time `json:"expires_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// data is what the ledger keeps of a share for the notification.
type data struct {
	Path         string    `json:"path"`
	File         bool      `json:"file"`
	Project      string    `json:"project,omitempty"`
	Received     bool      `json:"received,omitempty"`
	Sharer       string    `json:"sharer"`
	Grantee      string    `json:"grantee"`
	GranteeGroup bool      `json:"grantee_group,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	DaysLeft     int       `json:"days_left"`
}

// Scan yields the shares that expire before the furthest reminder point has
// passed, except the shares whose resource does not exist any more.
func (k *kind) Scan(ctx context.Context) iter.Seq2[reminders.Item, error] {
	return func(yield func(reminders.Item, error) bool) {
		gw, adm, err := k.clients(ctx)
		if err != nil {
			yield(reminders.Item{}, err)
			return
		}
		now := k.now()
		today := midnight(now)
		projects := map[string]*project{}
		args := map[string]string{
			"from":  now.Format(time.RFC3339),
			"to":    today.AddDate(0, 0, k.conf.DaysBefore[0]+1).Format(time.RFC3339),
			"limit": strconv.Itoa(pageSize),
		}
		for {
			shares, err := expiring(ctx, adm, args)
			if err != nil {
				yield(reminders.Item{}, err)
				return
			}
			if len(shares) == 0 {
				return
			}
			for _, s := range shares {
				items, err := k.items(ctx, gw, adm, projects, s, today)
				if err != nil {
					if !yield(reminders.Item{}, fmt.Errorf("share %d: %w", s.ID, err)) {
						return
					}
					continue
				}
				for _, it := range items {
					if !yield(it, nil) {
						return
					}
				}
			}
			last := shares[len(shares)-1]
			args["after_expiration"], args["after_id"] = last.ExpiresAt.Format(time.RFC3339Nano), strconv.FormatUint(uint64(last.ID), 10)
		}
	}
}

func (k *kind) clients(ctx context.Context) (gateway.GatewayAPIClient, adminpb.AdminAPIClient, error) {
	gw, adm := k.gateway, k.admin
	var err error
	if gw == nil {
		if gw, err = service.Gateway(ctx); err != nil {
			return nil, nil, err
		}
	}
	if adm == nil {
		if adm, err = service.Admin(ctx); err != nil {
			return nil, nil, err
		}
	}
	return gw, adm, nil
}

// expiring reads one page of shares from the user share provider.
func expiring(ctx context.Context, adm adminpb.AdminAPIClient, args map[string]string) ([]share, error) {
	var page struct {
		Shares []share `json:"shares"`
	}
	if err := invoke(ctx, adm, "usershareprovider", "expiring", args, &page); err != nil {
		return nil, fmt.Errorf("listing the expiring shares: %w", err)
	}
	return page.Shares, nil
}

// project is a project space as the space invocation of the spaces registry
// returns it.
type project struct {
	Name  string `json:"name"`
	Roles []struct {
		Name   string   `json:"name"`
		Users  []string `json:"users"`
		Groups []string `json:"groups"`
	} `json:"roles"`
}

// invoke calls an invocation of a service through the Admin API as the
// reminders service. Every instance reads the same database, so the first
// answer is enough.
func invoke(ctx context.Context, adm adminpb.AdminAPIClient, svc, invocation string, args map[string]string, out any) error {
	ctx, err := admin.AsService(ctx, "reminders")
	if err != nil {
		return err
	}
	res, err := adm.Invoke(ctx, &adminpb.InvokeRequest{Service: svc, Invocation: invocation, Args: args})
	if err != nil {
		return err
	}
	var failures []error
	for _, r := range res.GetResults() {
		if r.GetError() != "" {
			failures = append(failures, fmt.Errorf("%s: %s", r.GetNode(), r.GetError()))
			continue
		}
		return json.Unmarshal([]byte(r.GetResultJson()), out)
	}
	return fmt.Errorf("no %s answered: %w", svc, errors.Join(failures...))
}

// items returns the share once for each recipient. The sharer is the
// initiator for a share in a home, the users and groups of RecipientRole for a
// share in a project. A share whose resource is gone has no items.
func (k *kind) items(ctx context.Context, gw gateway.GatewayAPIClient, adm adminpb.AdminAPIClient, projects map[string]*project, s share, today time.Time) ([]reminders.Item, error) {
	res, err := gw.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: s.Instance, OpaqueId: s.Inode}}})
	if err != nil {
		return nil, err
	}
	switch res.GetStatus().GetCode() {
	case rpc.Code_CODE_OK:
	case rpc.Code_CODE_NOT_FOUND:
		return nil, nil
	default:
		return nil, fmt.Errorf("stat: %s", res.GetStatus().GetMessage())
	}

	expires := s.ExpiresAt.In(today.Location())
	daysLeft := int(math.Round(midnight(expires).Sub(today).Hours() / 24))
	level := k.level(daysLeft, midnight(expires), s.UpdatedAt)
	d := data{
		Path:         res.GetInfo().GetPath(),
		File:         s.ItemType == "file",
		Sharer:       s.Initiator,
		Grantee:      s.ShareWith,
		GranteeGroup: s.Group,
		ExpiresAt:    s.ExpiresAt,
		DaysLeft:     daysLeft,
	}

	var items []reminders.Item
	add := func(side string, to []reminders.Recipient, d data) error {
		b, err := json.Marshal(d)
		if err != nil {
			return err
		}
		for _, r := range to {
			items = append(items, reminders.Item{
				// The date comes first, so that the soonest share is first
				// in a level.
				Subject:   fmt.Sprintf("share:%s:%d:%s:%s", expires.Format(time.DateOnly), s.ID, side, r.Name),
				Level:     level,
				Recipient: r,
				Data:      b,
			})
		}
		return nil
	}

	if slices.Contains(k.conf.Notify, notifySharer) {
		to := []reminders.Recipient{{Name: s.Initiator}}
		sd := d
		if space := res.GetInfo().GetSpace(); space.GetSpaceType() == spaces.SpaceTypeProject.AsString() {
			id := space.GetId().GetOpaqueId()
			p, ok := projects[id]
			if !ok {
				p = &project{}
				if err := invoke(ctx, adm, "spacesregistry", "space", map[string]string{"id": id}, p); err != nil {
					return nil, fmt.Errorf("reading the roles of space %s: %w", id, err)
				}
				projects[id] = p
			}
			if r := k.recipients(p); len(r) > 0 {
				to, sd.Project = r, p.Name
			}
		}
		if err := add(notifySharer, to, sd); err != nil {
			return nil, err
		}
	}
	if slices.Contains(k.conf.Notify, notifyGrantee) {
		gd := d
		gd.Received = true
		if err := add(notifyGrantee, []reminders.Recipient{{Name: s.ShareWith, Group: s.Group}}, gd); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (k *kind) recipients(p *project) []reminders.Recipient {
	var to []reminders.Recipient
	for _, role := range p.Roles {
		if role.Name != k.conf.RecipientRole {
			continue
		}
		for _, u := range role.Users {
			to = append(to, reminders.Recipient{Name: u})
		}
		for _, g := range role.Groups {
			to = append(to, reminders.Recipient{Name: g, Group: true})
		}
	}
	return to
}

// level skips a reminder point whose window was already open when the share
// was last changed: the sharer chose that expiration.
func (k *kind) level(daysLeft int, expiryDay, updated time.Time) int {
	level := 0
	for i, d := range k.conf.DaysBefore {
		if daysLeft > d {
			break
		}
		if updated.Before(expiryDay.AddDate(0, 0, -d)) {
			level = i + 1
		}
	}
	return level
}

// Notice builds the template data of the share-expiry mail. It is kept to
// strings, numbers and booleans: the data travels as JSON, and the template
// functions do not compare a float with an integer literal.
func (k *kind) Notice(_ context.Context, to reminders.Recipient, items iter.Seq[reminders.Item]) (map[string]any, error) {
	var shared, received []map[string]any
	count, soonest, username, isProject := 0, math.MaxInt, to.Name, false
	for it := range items {
		var d data
		if err := json.Unmarshal(it.Data, &d); err != nil {
			return nil, fmt.Errorf("%s: %w", it.Subject, err)
		}
		count++
		soonest = min(soonest, d.DaysLeft)
		if d.Project != "" {
			username, isProject = d.Project, true
		}
		if count > k.conf.MaxSharesPerMail {
			continue
		}
		s := map[string]any{
			"Path":         d.Path,
			"DisplayPath":  displayPath(d.Path, 64),
			"Name":         path.Base(d.Path),
			"File":         d.File,
			"ExpiresAt":    d.ExpiresAt.Format(time.DateOnly),
			"DaysLeft":     d.DaysLeft,
			"DaysLeftText": daysText(d.DaysLeft),
		}
		if d.Received {
			s["Sharer"] = d.Sharer
			s["WebURL"] = k.conf.BaseURL + "/files/shares/with-me"
			received = append(received, s)
			continue
		}
		s["Grantee"], s["GranteeIsGroup"] = d.Grantee, d.GranteeGroup
		s["WebURL"] = k.conf.BaseURL + "/files/spaces" + d.Path
		if d.File {
			s["WebURL"] = k.conf.BaseURL + "/files/spaces" + path.Dir(d.Path) + "?scrollTo=" + path.Base(d.Path)
		}
		shared = append(shared, s)
	}
	if count == 0 {
		return nil, errors.New("no share to remind")
	}
	return map[string]any{
		"Username":     username,
		"IsProject":    isProject,
		"Soonest":      soonest,
		"SoonestText":  daysText(soonest),
		"ShareCount":   count,
		"Plural":       count > 1,
		"Shared":       shared,
		"HasShared":    len(shared) > 0,
		"Received":     received,
		"HasReceived":  len(received) > 0,
		"Remaining":    max(0, count-k.conf.MaxSharesPerMail),
		"HasRemaining": count > k.conf.MaxSharesPerMail,
	}, nil
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func daysText(days int) string {
	switch days {
	case 0:
		return "today"
	case 1:
		return "tomorrow"
	}
	return fmt.Sprintf("in %d days", days)
}

// displayPath shortens a path for the mail, and keeps both ends.
func displayPath(p string, maxChars int) string {
	if len(p) < maxChars {
		return p
	}
	left := (maxChars - 3) / 2
	return p[:left] + "..." + p[len(p)-(maxChars-3-left):]
}
