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

package sql

import (
	"context"
	"time"

	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/cs3org/reva/v3/pkg/stats"
	"gorm.io/gorm"
)

// The SQL share managers implement stats.Reporter: aggregate GROUP BY
// queries over the share tables, so the schema knowledge stays inside the
// driver that owns it. The services detect the capability and expose it
// as the "stats" invocation; the stats prometheus collector turns the
// payloads into metrics.

// statusCase buckets each row into exactly one status. Priority: deleted
// wins over orphan, orphan over expired, expired over active — so counts
// sum to the total.
const statusCase = `CASE
  WHEN deleted_at IS NOT NULL THEN 'deleted'
  WHEN orphan OR orphaned_at IS NOT NULL THEN 'orphan'
  WHEN expiration IS NOT NULL AND expiration < ? THEN 'expired'
  ELSE 'active'
END`

// permissionClass maps the OCS permission bitmask to a small vocabulary:
// read-only, read-write, upload-only (write bits without read), none.
func permissionClass(p uint8) string {
	const write = permissions.PermissionWrite | permissions.PermissionCreate | permissions.PermissionDelete
	perms := permissions.OcsPermissions(p)
	canRead := perms.Contain(permissions.PermissionRead)
	canWrite := perms&write != 0
	switch {
	case canRead && canWrite:
		return "read-write"
	case canRead:
		return "read"
	case canWrite:
		return "upload-only"
	default:
		return "none"
	}
}

// shareCountRow is one aggregate bucket of the by-dimensions query.
type shareCountRow struct {
	Status      string
	ItemType    string
	Instance    string
	Permissions uint8
	Count       int64
}

// countsByDimensions aggregates a share table into (status, item_type,
// instance, permission) buckets, mapping the permission bitmask in Go.
func countsByDimensions(db *gorm.DB, tbl any, kind string) (stats.Metric, error) {
	var rows []shareCountRow
	err := db.Model(tbl).Unscoped().
		Select(statusCase+" AS status, item_type, instance, permissions, COUNT(*) AS count", time.Now()).
		Group("status, item_type, instance, permissions").
		Find(&rows).Error
	if err != nil {
		return stats.Metric{}, err
	}

	// fold the permission bitmask into its class
	folded := map[[4]string]int64{}
	for _, r := range rows {
		key := [4]string{r.Status, r.ItemType, r.Instance, permissionClass(r.Permissions)}
		folded[key] += r.Count
	}
	m := stats.Metric{
		Name: "shares",
		Help: "Shares by kind, status, item type, storage instance and permission class.",
		Kind: stats.KindGauge,
	}
	for key, count := range folded {
		m.Samples = append(m.Samples, stats.Sample{
			Labels: map[string]string{
				"kind":       kind,
				"status":     key[0],
				"item_type":  key[1],
				"instance":   key[2],
				"permission": key[3],
			},
			Value: float64(count),
		})
	}
	return m, nil
}

// creators counts the distinct initiators of live shares.
func creators(db *gorm.DB, tbl any, kind string) (stats.Metric, error) {
	var count int64
	if err := db.Model(tbl).Distinct("uid_initiator").Count(&count).Error; err != nil {
		return stats.Metric{}, err
	}
	return stats.Metric{
		Name: "share_creators",
		Help: "Distinct users having created shares.",
		Kind: stats.KindGauge,
		Samples: []stats.Sample{
			{Labels: map[string]string{"kind": kind}, Value: float64(count)},
		},
	}, nil
}

// activeByCreator counts the active shares of each creator. The samples are
// per owner, so the collector can fold them by the creator's attributes.
func activeByCreator(db *gorm.DB, tbl any, kind string) ([]stats.Metric, error) {
	type row struct {
		UIDInitiator string
		Count        int64
	}
	var rows []row
	err := db.Model(tbl).
		Select("uid_initiator, COUNT(*) AS count").
		Where("NOT (orphan OR orphaned_at IS NOT NULL) AND (expiration IS NULL OR expiration >= ?)", time.Now()).
		Group("uid_initiator").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	labels := map[string]string{"kind": kind}
	shares := stats.Metric{
		Name: "active_shares",
		Help: "Active shares, by kind and creator attributes.",
		Kind: stats.KindGauge,
	}
	creators := stats.Metric{
		Name: "active_share_creators",
		Help: "Distinct users having active shares, by kind and creator attributes.",
		Kind: stats.KindGauge,
	}
	for _, r := range rows {
		// An owner sample needs an owner; keep the count of old rows without one.
		if r.UIDInitiator == "" {
			shares.Samples = append(shares.Samples, stats.Sample{Labels: labels, Value: float64(r.Count)})
			continue
		}
		shares.OwnerSamples = append(shares.OwnerSamples, stats.OwnerSample{Owner: r.UIDInitiator, Labels: labels, Value: float64(r.Count)})
		creators.OwnerSamples = append(creators.OwnerSamples, stats.OwnerSample{Owner: r.UIDInitiator, Labels: labels, Value: 1})
	}
	return []stats.Metric{shares, creators}, nil
}

// Stats implements stats.Reporter for the user/group share manager.
func (m *ShareMgr) Stats(ctx context.Context) (*stats.Payload, error) {
	db := m.db.WithContext(ctx)
	p := &stats.Payload{}

	byDim, err := countsByDimensions(db, &model.Share{}, "share")
	if err != nil {
		return nil, err
	}
	creat, err := creators(db, &model.Share{}, "share")
	if err != nil {
		return nil, err
	}

	// distinct recipients, by recipient type
	type recipientRow struct {
		SharedWithIsGroup bool
		Count             int64
	}
	var recRows []recipientRow
	err = db.Model(&model.Share{}).
		Select("shared_with_is_group, COUNT(DISTINCT share_with) AS count").
		Group("shared_with_is_group").
		Find(&recRows).Error
	if err != nil {
		return nil, err
	}
	recipients := stats.Metric{
		Name: "share_recipients",
		Help: "Distinct share recipients, by recipient type.",
		Kind: stats.KindGauge,
	}
	for _, r := range recRows {
		rtype := "user"
		if r.SharedWithIsGroup {
			rtype = "group"
		}
		recipients.Samples = append(recipients.Samples, stats.Sample{
			Labels: map[string]string{"recipient_type": rtype},
			Value:  float64(r.Count),
		})
	}

	// shares per recipient: max and average
	type distRow struct {
		Max float64
		Avg float64
	}
	var dist distRow
	sub := db.Model(&model.Share{}).Select("COUNT(*) AS c").Group("share_with")
	err = db.Table("(?) AS per_recipient", sub).
		Select("MAX(c) AS max, AVG(c) AS avg").
		Find(&dist).Error
	if err != nil {
		return nil, err
	}
	perRecipientMax := stats.Metric{
		Name:    "shares_per_recipient_max",
		Help:    "Maximum number of shares received by a single recipient.",
		Kind:    stats.KindGauge,
		Samples: []stats.Sample{{Value: dist.Max}},
	}
	perRecipientAvg := stats.Metric{
		Name:    "shares_per_recipient_avg",
		Help:    "Average number of shares received per recipient.",
		Kind:    stats.KindGauge,
		Samples: []stats.Sample{{Value: dist.Avg}},
	}

	active, err := activeByCreator(db, &model.Share{}, "share")
	if err != nil {
		return nil, err
	}

	p.Metrics = append(p.Metrics, byDim, creat, recipients, perRecipientMax, perRecipientAvg)
	p.Metrics = append(p.Metrics, active...)
	return p, nil
}

// Stats implements stats.Reporter for the public link manager.
func (m *PublicShareMgr) Stats(ctx context.Context) (*stats.Payload, error) {
	db := m.db.WithContext(ctx)
	p := &stats.Payload{}

	byDim, err := countsByDimensions(db, &model.PublicLink{}, "link")
	if err != nil {
		return nil, err
	}
	creat, err := creators(db, &model.PublicLink{}, "link")
	if err != nil {
		return nil, err
	}

	// live links by protection and quicklink flag
	type linkRow struct {
		Protected bool
		Quicklink bool
		Count     int64
	}
	var linkRows []linkRow
	err = db.Model(&model.PublicLink{}).
		Select("password <> '' AS protected, quicklink, COUNT(*) AS count").
		Group("protected, quicklink").
		Find(&linkRows).Error
	if err != nil {
		return nil, err
	}
	links := stats.Metric{
		Name: "links",
		Help: "Public links by password protection and quicklink flag.",
		Kind: stats.KindGauge,
	}
	for _, r := range linkRows {
		links.Samples = append(links.Samples, stats.Sample{
			Labels: map[string]string{
				"password_protected": boolLabel(r.Protected),
				"quicklink":          boolLabel(r.Quicklink),
			},
			Value: float64(r.Count),
		})
	}

	active, err := activeByCreator(db, &model.PublicLink{}, "link")
	if err != nil {
		return nil, err
	}

	p.Metrics = append(p.Metrics, byDim, creat, links)
	p.Metrics = append(p.Metrics, active...)
	return p, nil
}

func boolLabel(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
