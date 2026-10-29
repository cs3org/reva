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

package reminders

import (
	"context"
	"encoding/json"
	"iter"
	"maps"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// claimTimeout is how long a claim holds. A claim that is older belongs to a
// run that stopped between the claim and the end of the notification, and
// another run can take it.
const claimTimeout = time.Hour

const pageSize = 500

// Ledger is what the reminders remember: the open episodes, the reminders
// sent and the last run of each kind. Times are kept in UTC, because sqlite
// compares them as text.
type Ledger struct {
	DB *gorm.DB
}

// Episode is a subject whose condition holds.
type Episode struct {
	ID             uint           `gorm:"primaryKey" json:"-"`
	Kind           string         `gorm:"size:64;uniqueIndex:idx_reminders_state_subject;index:idx_reminders_state_claim,priority:1" json:"kind"`
	Subject        string         `gorm:"size:255;uniqueIndex:idx_reminders_state_subject" json:"subject"`
	Recipient      string         `gorm:"size:255;index" json:"recipient"`
	RecipientGroup bool           `json:"recipient_group"`
	LevelSent      int            `json:"level_sent"`
	ClaimedBy      *string        `gorm:"size:36;index:idx_reminders_state_claim,priority:2" json:"claimed_by,omitempty"`
	ClaimedLevel   int            `json:"-"`
	ClaimedData    datatypes.JSON `json:"-"`
	ClaimedAt      *time.Time     `json:"claimed_at,omitempty"`
	OpenedAt       time.Time      `json:"opened_at"`
	SeenAt         time.Time      `json:"seen_at"`
}

// TableName implements gorm's tabler.
func (Episode) TableName() string { return "reminders_state" }

// Sent is a reminder sent.
type Sent struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	Kind      string    `gorm:"size:64;index" json:"kind"`
	Subject   string    `gorm:"size:255" json:"subject"`
	Level     int       `json:"level"`
	Recipient string    `gorm:"size:255;index" json:"recipient"`
	Event     string    `gorm:"size:64" json:"event"`
	SentAt    time.Time `gorm:"index" json:"sent_at"`
}

// TableName implements gorm's tabler.
func (Sent) TableName() string { return "reminders_sent" }

// RunRecord is the last run of a kind. The jobs runner keeps no status for
// periodic runs.
type RunRecord struct {
	Kind       string         `gorm:"primaryKey;size:64" json:"kind"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
	Report     datatypes.JSON `json:"report"`
	Error      string         `gorm:"size:1024" json:"error,omitempty"`
}

// TableName implements gorm's tabler.
func (RunRecord) TableName() string { return "reminders_runs" }

// Filter selects episodes or reminders sent. Empty fields select all.
type Filter struct {
	Kind      string
	Subject   string
	Recipient string
	// Since applies to the reminders sent only.
	Since time.Time
	Limit int
}

// Summary counts the state of one kind.
type Summary struct {
	Open    int64 `json:"open"`
	Pending int64 `json:"pending"`
	Sent24h int64 `json:"sent_24h"`
	Sent7d  int64 `json:"sent_7d"`
}

// Migrate creates or updates the tables.
func (l *Ledger) Migrate() error {
	return l.DB.AutoMigrate(&Episode{}, &Sent{}, &RunRecord{})
}

// Observe opens an episode for each new subject and records that every
// subject was seen at now.
func (l *Ledger) Observe(ctx context.Context, kind string, items []Item, now time.Time) error {
	if len(items) == 0 {
		return nil
	}
	rows := make([]Episode, 0, len(items))
	for _, it := range items {
		rows = append(rows, Episode{Kind: kind, Subject: it.Subject, Recipient: it.Recipient.Name, RecipientGroup: it.Recipient.Group, OpenedAt: now, SeenAt: now})
	}
	return l.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "kind"}, {Name: "subject"}},
		DoUpdates: clause.AssignmentColumns([]string{"recipient", "recipient_group", "seen_at"}),
	}).Create(&rows).Error
}

// LevelsSent returns the level sent in the episode of each subject that has
// one.
func (l *Ledger) LevelsSent(ctx context.Context, kind string, subjects []string) (map[string]int, error) {
	var rows []Episode
	if err := l.DB.WithContext(ctx).Select("subject", "level_sent").Where("kind = ? AND subject IN ?", kind, subjects).Find(&rows).Error; err != nil {
		return nil, err
	}
	sent := make(map[string]int, len(rows))
	for _, e := range rows {
		sent[e.Subject] = e.LevelSent
	}
	return sent, nil
}

// Claim gives the run the items whose level is higher than the level sent in
// their episode, if no other run holds them. The items must be observed. It
// returns how many items the run got.
func (l *Ledger) Claim(ctx context.Context, kind, run string, items []Item, now time.Time) (int, error) {
	subjects := make([]string, 0, len(items))
	for _, it := range items {
		subjects = append(subjects, it.Subject)
	}
	sent, err := l.LevelsSent(ctx, kind, subjects)
	if err != nil {
		return 0, err
	}
	claimed := 0
	err = l.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, it := range items {
			if it.Level <= sent[it.Subject] {
				continue
			}
			res := tx.Model(&Episode{}).
				Where("kind = ? AND subject = ? AND level_sent < ?", kind, it.Subject, it.Level).
				Where("(claimed_by IS NULL OR claimed_at < ?)", now.Add(-claimTimeout)).
				Updates(map[string]any{"claimed_by": run, "claimed_level": it.Level, "claimed_data": datatypes.JSON(it.Data), "claimed_at": now})
			if res.Error != nil {
				return res.Error
			}
			claimed += int(res.RowsAffected)
		}
		return nil
	})
	return claimed, err
}

// Recipients yields the recipients of the items that the run claimed.
func (l *Ledger) Recipients(ctx context.Context, kind, run string) iter.Seq2[Recipient, error] {
	return func(yield func(Recipient, error) bool) {
		var last *Recipient
		for {
			q := l.DB.WithContext(ctx).Model(&Episode{}).Distinct("recipient", "recipient_group").Where("kind = ? AND claimed_by = ?", kind, run)
			if last != nil {
				q = q.Where("(recipient > ? OR (recipient = ? AND recipient_group > ?))", last.Name, last.Name, last.Group)
			}
			var page []Episode
			if err := q.Order("recipient, recipient_group").Limit(pageSize).Find(&page).Error; err != nil {
				yield(Recipient{}, err)
				return
			}
			for _, e := range page {
				to := Recipient{Name: e.Recipient, Group: e.RecipientGroup}
				if !yield(to, nil) {
					return
				}
				last = &to
			}
			if len(page) < pageSize {
				return
			}
		}
	}
}

// Claimed yields the items that the run claimed for a recipient, highest level
// first, then by subject.
func (l *Ledger) Claimed(ctx context.Context, kind, run string, to Recipient) iter.Seq2[Item, error] {
	return func(yield func(Item, error) bool) {
		rows, err := claims(l.DB.WithContext(ctx), kind, run, &to).
			Select("subject", "claimed_level", "claimed_data").
			Order("claimed_level DESC, subject").Rows()
		if err != nil {
			yield(Item{}, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var e Episode
			if err := l.DB.ScanRows(rows, &e); err != nil {
				yield(Item{}, err)
				return
			}
			if !yield(Item{Subject: e.Subject, Level: e.ClaimedLevel, Recipient: to, Data: json.RawMessage(e.ClaimedData)}, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(Item{}, err)
		}
	}
}

// MarkSent records that the items claimed by the run for a recipient were
// sent, and ends their claim.
func (l *Ledger) MarkSent(ctx context.Context, kind, run string, to Recipient, event string, now time.Time) error {
	return l.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Exec("INSERT INTO reminders_sent (kind, subject, level, recipient, event, sent_at) "+
			"SELECT kind, subject, claimed_level, recipient, ?, ? FROM reminders_state "+
			"WHERE kind = ? AND claimed_by = ? AND recipient = ? AND recipient_group = ?",
			event, now, kind, run, to.Name, to.Group).Error
		if err != nil {
			return err
		}
		// claimed_level is not reset: MySQL assigns from left to right, so
		// level_sent would read the new value.
		updates := map[string]any{"level_sent": gorm.Expr("claimed_level")}
		maps.Copy(updates, unclaimed)
		return claims(tx, kind, run, &to).Updates(updates).Error
	})
}

// Release ends the claims of the run, for one recipient or for all when to is
// nil, without recording anything as sent.
func (l *Ledger) Release(ctx context.Context, kind, run string, to *Recipient) error {
	return claims(l.DB.WithContext(ctx), kind, run, to).Updates(unclaimed).Error
}

var unclaimed = map[string]any{"claimed_by": nil, "claimed_data": nil, "claimed_at": nil}

func claims(db *gorm.DB, kind, run string, to *Recipient) *gorm.DB {
	q := db.Model(&Episode{}).Where("kind = ? AND claimed_by = ?", kind, run)
	if to != nil {
		q = q.Where("recipient = ? AND recipient_group = ?", to.Name, to.Group)
	}
	return q
}

// Close deletes the episodes of a kind not seen since before.
func (l *Ledger) Close(ctx context.Context, kind string, before time.Time) (int64, error) {
	res := l.DB.WithContext(ctx).Where("kind = ? AND seen_at < ? AND claimed_by IS NULL", kind, before.UTC()).Delete(&Episode{})
	return res.RowsAffected, res.Error
}

// Forget deletes an episode, so that its subject is told again from the
// lowest level.
func (l *Ledger) Forget(ctx context.Context, kind, subject string) (int64, error) {
	res := l.DB.WithContext(ctx).Where("kind = ? AND subject = ?", kind, subject).Delete(&Episode{})
	return res.RowsAffected, res.Error
}

// Purge deletes the reminders sent before a time.
func (l *Ledger) Purge(ctx context.Context, before time.Time) (int64, error) {
	res := l.DB.WithContext(ctx).Where("sent_at < ?", before.UTC()).Delete(&Sent{})
	return res.RowsAffected, res.Error
}

// Episodes returns the open episodes.
func (l *Ledger) Episodes(ctx context.Context, f Filter) ([]Episode, error) {
	var out []Episode
	err := l.filter(ctx, f).Order("kind, recipient, subject").Find(&out).Error
	return out, err
}

// History returns the reminders sent, the most recent first.
func (l *Ledger) History(ctx context.Context, f Filter) ([]Sent, error) {
	q := l.filter(ctx, f)
	if !f.Since.IsZero() {
		q = q.Where("sent_at >= ?", f.Since.UTC())
	}
	var out []Sent
	err := q.Order("sent_at DESC, id DESC").Find(&out).Error
	return out, err
}

// Summary counts the episodes and the reminders sent of a kind.
func (l *Ledger) Summary(ctx context.Context, kind string, now time.Time) (Summary, error) {
	var s Summary
	db := l.DB.WithContext(ctx)
	now = now.UTC()
	for _, q := range []struct {
		n     *int64
		model any
		where string
		args  []any
	}{
		{&s.Open, &Episode{}, "kind = ?", []any{kind}},
		{&s.Pending, &Episode{}, "kind = ? AND claimed_by IS NOT NULL", []any{kind}},
		{&s.Sent24h, &Sent{}, "kind = ? AND sent_at >= ?", []any{kind, now.Add(-24 * time.Hour)}},
		{&s.Sent7d, &Sent{}, "kind = ? AND sent_at >= ?", []any{kind, now.Add(-7 * 24 * time.Hour)}},
	} {
		if err := db.Model(q.model).Where(q.where, q.args...).Count(q.n).Error; err != nil {
			return s, err
		}
	}
	return s, nil
}

func (l *Ledger) filter(ctx context.Context, f Filter) *gorm.DB {
	q := l.DB.WithContext(ctx)
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.Subject != "" {
		q = q.Where("subject = ?", f.Subject)
	}
	if f.Recipient != "" {
		q = q.Where("recipient = ?", f.Recipient)
	}
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	return q
}

// RecordRun keeps r as the last run of its kind.
func (l *Ledger) RecordRun(ctx context.Context, r RunRecord) error {
	return l.DB.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&r).Error
}

// Runs returns the last run of every kind.
func (l *Ledger) Runs(ctx context.Context) ([]RunRecord, error) {
	var out []RunRecord
	err := l.DB.WithContext(ctx).Find(&out).Error
	return out, err
}
