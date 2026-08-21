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

// Package sql is a GORM-backed store for login flow enrolments. It runs the
// atomic state transitions approve and consume as compare-and-set updates whose
// affected-row count decides the winner of a race.
package sql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appauth/loginflow"
	"github.com/cs3org/reva/v3/pkg/appauth/loginflow/registry"
	"github.com/cs3org/reva/v3/pkg/appauth/loginflow/sql/model"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	registry.Register("sql", New)
}

// Config holds the store configuration.
type Config struct {
	config.Database `mapstructure:",squash"`
}

func (c *Config) ApplyDefaults() {
	c.Database = sharedconf.GetDBInfo(c.Database)
}

type mgr struct {
	db *gorm.DB
}

// New returns a GORM-backed login flow store.
func New(ctx context.Context, m map[string]any) (loginflow.Manager, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	db, err := getDB(c)
	if err != nil {
		return nil, fmt.Errorf("loginflow sql: opening database: %w", err)
	}
	if err := db.AutoMigrate(&model.Flow{}); err != nil {
		return nil, fmt.Errorf("loginflow sql: migrating schema: %w", err)
	}

	return &mgr{db: db}, nil
}

func getDB(c Config) (*gorm.DB, error) {
	gormCfg := &gorm.Config{}
	switch c.Engine {
	case "sqlite":
		return gorm.Open(sqlite.Open(c.DBName), gormCfg)
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		return gorm.Open(mysql.Open(dsn), gormCfg)
	default:
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		return gorm.Open(mysql.Open(dsn), gormCfg)
	}
}

func (m *mgr) CreateFlow(ctx context.Context, f *loginflow.Flow) error {
	row := &model.Flow{
		LoginHash: f.LoginHash,
		PollHash:  f.PollHash,
		ClientID:  f.ClientID,
		UserAgent: f.UserAgent,
		ExpiresAt: f.ExpiresAt,
	}
	if err := m.db.WithContext(ctx).Create(row).Error; err != nil {
		return err
	}
	f.CreatedAt = row.CreatedAt
	return nil
}

func (m *mgr) GetByLogin(ctx context.Context, loginHash []byte) (*loginflow.Flow, error) {
	return m.getBy(ctx, "login_hash = ?", loginHash)
}

func (m *mgr) GetByPoll(ctx context.Context, pollHash []byte) (*loginflow.Flow, error) {
	return m.getBy(ctx, "poll_hash = ?", pollHash)
}

// getBy returns a live (non-deleted) flow, expired or not, so the caller can
// tell "gone" from "unknown".
func (m *mgr) getBy(ctx context.Context, query string, arg any) (*loginflow.Flow, error) {
	var row model.Flow
	err := m.db.WithContext(ctx).Where(query, arg).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errtypes.NotFound("loginflow: flow not found")
	}
	if err != nil {
		return nil, err
	}
	return toFlow(&row), nil
}

func (m *mgr) Approve(ctx context.Context, loginHash []byte, userID, username, deviceName string) error {
	now := time.Now()
	res := m.db.WithContext(ctx).
		Model(&model.Flow{}).
		Where("login_hash = ? AND approved_at IS NULL AND expires_at > ?", loginHash, now).
		Updates(map[string]any{
			"approved_at": now,
			"user_id":     userID,
			"username":    username,
			"device_name": deviceName,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errtypes.Conflict("loginflow: flow not pending")
	}
	return nil
}

func (m *mgr) Consume(ctx context.Context, pollHash []byte) (*loginflow.Flow, error) {
	var consumed *loginflow.Flow
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.Flow
		err := tx.Where("poll_hash = ? AND approved_at IS NOT NULL AND expires_at > ?", pollHash, time.Now()).
			First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errtypes.NotFound("loginflow: flow not approved")
		}
		if err != nil {
			return err
		}

		// Soft-delete with the same predicate; RowsAffected != 1 means another
		// poll consumed it first.
		res := tx.Where("poll_hash = ? AND approved_at IS NOT NULL AND expires_at > ?", pollHash, time.Now()).
			Delete(&model.Flow{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errtypes.NotFound("loginflow: flow already consumed")
		}

		consumed = toFlow(&row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return consumed, nil
}

func (m *mgr) Deny(ctx context.Context, loginHash []byte) error {
	res := m.db.WithContext(ctx).
		Where("login_hash = ? AND approved_at IS NULL", loginHash).
		Delete(&model.Flow{})
	return res.Error
}

func toFlow(row *model.Flow) *loginflow.Flow {
	return &loginflow.Flow{
		LoginHash:  row.LoginHash,
		PollHash:   row.PollHash,
		ClientID:   row.ClientID,
		UserAgent:  row.UserAgent,
		CreatedAt:  row.CreatedAt,
		ExpiresAt:  row.ExpiresAt,
		ApprovedAt: row.ApprovedAt,
		UserID:     row.UserID,
		Username:   row.Username,
		DeviceName: row.DeviceName,
	}
}
