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

// Package sql is a GORM-backed appauth manager. App passwords are stored as
// SHA-256 hashes and looked up directly, without a bcrypt loop, because the
// tokens carry 256 bits of entropy. last_seen (utime) writes are buffered so
// every request does not touch the database.
package sql

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	apppb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appauth"
	"github.com/cs3org/reva/v3/pkg/appauth/manager/registry"
	"github.com/cs3org/reva/v3/pkg/appauth/manager/sql/model"
	"github.com/cs3org/reva/v3/pkg/appctx"
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
	config.Database   `mapstructure:",squash"`
	UtimeFlushSeconds int `mapstructure:"utime_flush_seconds"` // buffer window for last_seen
}

func (c *Config) ApplyDefaults() {
	c.Database = sharedconf.GetDBInfo(c.Database)
	if c.UtimeFlushSeconds == 0 {
		c.UtimeFlushSeconds = 300
	}
}

type mgr struct {
	db    *gorm.DB
	flush time.Duration

	mu       sync.Mutex
	lastSeen map[string]time.Time // hex(token hash) -> last persisted utime
}

// New returns a GORM-backed appauth manager.
func New(ctx context.Context, m map[string]any) (appauth.Manager, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	db, err := getDB(c)
	if err != nil {
		return nil, fmt.Errorf("appauth sql: opening database: %w", err)
	}
	if err := db.AutoMigrate(&model.AppPassword{}); err != nil {
		return nil, fmt.Errorf("appauth sql: migrating schema: %w", err)
	}

	return &mgr{
		db:       db,
		flush:    time.Duration(c.UtimeFlushSeconds) * time.Second,
		lastSeen: make(map[string]time.Time),
	}, nil
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

func (m *mgr) GenerateAppPassword(ctx context.Context, scope map[string]*authpb.Scope, label string, expiration *typespb.Timestamp) (*apppb.AppPassword, error) {
	token, err := generateToken()
	if err != nil {
		return nil, err
	}
	userID := appctx.ContextMustGetUser(ctx).GetId()

	scopeJSON, err := json.Marshal(scope)
	if err != nil {
		return nil, fmt.Errorf("appauth sql: encoding scope: %w", err)
	}

	now := time.Now()
	row := &model.AppPassword{
		UserID:     userID.GetOpaqueId(),
		TokenHash:  hashToken(token),
		Label:      label,
		ScopeJSON:  scopeJSON,
		Ctime:      now,
		Utime:      now,
		Expiration: toTime(expiration),
	}
	if err := m.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}

	pw := toAppPassword(row, userID)
	pw.Password = token // return the plaintext exactly once
	return pw, nil
}

func (m *mgr) ListAppPasswords(ctx context.Context) ([]*apppb.AppPassword, error) {
	userID := appctx.ContextMustGetUser(ctx).GetId()

	var rows []model.AppPassword
	if err := m.db.WithContext(ctx).Where("user_id = ?", userID.GetOpaqueId()).Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]*apppb.AppPassword, 0, len(rows))
	for i := range rows {
		out = append(out, toAppPassword(&rows[i], userID))
	}
	return out, nil
}

func (m *mgr) InvalidateAppPassword(ctx context.Context, secret string) error {
	userID := appctx.ContextMustGetUser(ctx).GetId()

	// The management API and self-revocation pass the hash handle returned by
	// ListAppPasswords (hex of the token hash), not the plaintext.
	hash, err := hex.DecodeString(secret)
	if err != nil {
		return errtypes.NotFound("password not found")
	}

	res := m.db.WithContext(ctx).
		Where("user_id = ? AND token_hash = ?", userID.GetOpaqueId(), hash).
		Delete(&model.AppPassword{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errtypes.NotFound("password not found")
	}
	return nil
}

func (m *mgr) GetAppPassword(ctx context.Context, userID *userpb.UserId, secret string) (*apppb.AppPassword, error) {
	hash := hashToken(secret)

	var row model.AppPassword
	err := m.db.WithContext(ctx).
		Where("user_id = ? AND token_hash = ?", userID.GetOpaqueId(), hash).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errtypes.NotFound("password not found")
	}
	if err != nil {
		return nil, err
	}

	if row.Expiration != nil && time.Now().After(*row.Expiration) {
		return nil, errtypes.NotFound("password not found")
	}

	now := time.Now()
	m.touch(ctx, hash, now)

	pw := toAppPassword(&row, userID)
	pw.Utime = &typespb.Timestamp{Seconds: uint64(now.Unix())} // report accurate last-seen
	return pw, nil
}

// touch persists utime at most once per flush window per token.
func (m *mgr) touch(ctx context.Context, hash []byte, now time.Time) {
	key := hex.EncodeToString(hash)

	m.mu.Lock()
	last, ok := m.lastSeen[key]
	if ok && now.Sub(last) < m.flush {
		m.mu.Unlock()
		return
	}
	m.lastSeen[key] = now
	m.mu.Unlock()

	if err := m.db.WithContext(ctx).
		Model(&model.AppPassword{}).
		Where("token_hash = ?", hash).
		Update("utime", now).Error; err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("appauth sql: could not flush utime")
	}
}

// Helpers -----------------------------------------------------------------

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func toAppPassword(row *model.AppPassword, userID *userpb.UserId) *apppb.AppPassword {
	var scope map[string]*authpb.Scope
	if len(row.ScopeJSON) > 0 {
		_ = json.Unmarshal(row.ScopeJSON, &scope)
	}
	pw := &apppb.AppPassword{
		Password:   hex.EncodeToString(row.TokenHash), // stable handle, not the secret
		TokenScope: scope,
		Label:      row.Label,
		Ctime:      &typespb.Timestamp{Seconds: uint64(row.Ctime.Unix())},
		Utime:      &typespb.Timestamp{Seconds: uint64(row.Utime.Unix())},
		User:       userID,
	}
	if row.Expiration != nil {
		pw.Expiration = &typespb.Timestamp{Seconds: uint64(row.Expiration.Unix())}
	}
	return pw
}

func toTime(ts *typespb.Timestamp) *time.Time {
	if ts == nil || ts.Seconds == 0 {
		return nil
	}
	t := time.Unix(int64(ts.Seconds), int64(ts.Nanos)).UTC()
	return &t
}
