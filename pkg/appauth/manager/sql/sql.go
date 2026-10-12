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
	"errors"
	"fmt"
	"slices"
	"strings"
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
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/google/uuid"
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
	UtimeFlushSeconds int    `mapstructure:"utime_flush_seconds"` // buffer window for last_seen
	DefaultExpiryDays int    `mapstructure:"default_expiry_days" docs:"90;Expiry of an app password created without one, for the kinds of client that need one (all but sync)."`
	MaxExpiryDays     int    `mapstructure:"max_expiry_days" docs:"365;Longest expiry an app password can be created with."`
	Policy            Policy `mapstructure:"policy"`
}

// Policy says who can create app passwords.
type Policy struct {
	// UserTypes lists, per user type (primary, secondary, service, lightweight,
	// guest, federated, ...), the kinds of client its accounts can create app
	// passwords for. A type that is not listed can create none.
	UserTypes map[string][]string `mapstructure:"user_types"`
}

func (c *Config) ApplyDefaults() {
	c.Database = sharedconf.GetDBInfo(c.Database)
	if c.UtimeFlushSeconds == 0 {
		c.UtimeFlushSeconds = 300
	}
	if c.DefaultExpiryDays == 0 {
		c.DefaultExpiryDays = 90
	}
	if c.MaxExpiryDays == 0 {
		c.MaxExpiryDays = 365
	}
	if c.Policy.UserTypes == nil {
		c.Policy.UserTypes = map[string][]string{}
	}
	for _, t := range []string{"primary", "secondary", "service"} {
		if _, ok := c.Policy.UserTypes[t]; !ok {
			c.Policy.UserTypes[t] = scope.ClientKinds()
		}
	}
}

type mgr struct {
	c     Config
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
	if c.DefaultExpiryDays > c.MaxExpiryDays {
		return nil, fmt.Errorf("appauth sql: default_expiry_days exceeds max_expiry_days")
	}

	db, err := getDB(c)
	if err != nil {
		return nil, fmt.Errorf("appauth sql: opening database: %w", err)
	}
	if err := db.AutoMigrate(&model.AppPassword{}); err != nil {
		return nil, fmt.Errorf("appauth sql: migrating schema: %w", err)
	}
	if err := migrateLegacyRows(ctx, db); err != nil {
		return nil, fmt.Errorf("appauth sql: migrating app passwords: %w", err)
	}

	return &mgr{
		c:        c,
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

// legacyRows selects the rows written before app passwords had a client kind
// and permissions: those created before the migration, and those still
// written by older nodes during a rolling upgrade.
const legacyRows = "password_id IS NULL OR password_id = ''"

// migrateLegacyRows turns the rows written before app passwords had a client
// kind into sync clients with every permission, which is what they were.
func migrateLegacyRows(ctx context.Context, db *gorm.DB) error {
	var rows []model.AppPassword
	if err := db.WithContext(ctx).Where(legacyRows).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		if err := migrateRow(ctx, db, &rows[i]); err != nil {
			return err
		}
	}
	return nil
}

func migrateRow(ctx context.Context, db *gorm.DB, row *model.AppPassword) error {
	err := db.WithContext(ctx).Model(&model.AppPassword{}).
		Where("id = ?", row.ID).Where(legacyRows).
		Updates(&model.AppPassword{
			PasswordID:  uuid.NewString(),
			Client:      scope.ClientSync,
			Permissions: []string{scope.PermAll},
			CreatedBy:   row.UserID,
		}).Error
	if err != nil {
		return err
	}
	// Another node may have migrated it first.
	return db.WithContext(ctx).First(row, row.ID).Error
}

func (m *mgr) GenerateAppPassword(ctx context.Context, scopes map[string]*authpb.Scope, label string, expiration *typespb.Timestamp) (*apppb.AppPassword, error) {
	owner := appctx.ContextMustGetUser(ctx)

	ap, ok := scope.GetAppPasswordScope(scopes)
	if !ok || len(scopes) != 1 {
		return nil, errtypes.BadRequest("an app password carries exactly one apppassword scope")
	}
	if err := ap.Validate(); err != nil {
		return nil, err
	}
	// A consumer that lists shares or spaces has to filter them against the
	// roots, which only the MCP service does.
	if len(ap.Roots) > 0 && ap.Client != scope.ClientMCP {
		return nil, errtypes.BadRequest("roots are only supported for app passwords of the mcp kind")
	}
	if err := m.c.Policy.check(owner.GetId().GetType(), ap.Client); err != nil {
		return nil, err
	}
	expires, err := m.expiry(ap.Client, expiration)
	if err != nil {
		return nil, err
	}

	token, err := generateToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	row := &model.AppPassword{
		PasswordID:  uuid.NewString(),
		UserID:      owner.GetId().GetOpaqueId(),
		TokenHash:   hashToken(token),
		Label:       label,
		Client:      ap.Client,
		Permissions: ap.Permissions,
		Roots:       ap.Roots,
		CreatedBy:   owner.GetId().GetOpaqueId(),
		Ctime:       now,
		Utime:       now,
		Expiration:  expires,
	}
	if err := m.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}

	pw, err := toAppPassword(row, owner.GetId())
	if err != nil {
		return nil, err
	}
	pw.Password = token // return the plaintext exactly once
	return pw, nil
}

func (p Policy) check(t userpb.UserType, client string) error {
	name := strings.ToLower(strings.TrimPrefix(t.String(), "USER_TYPE_"))
	if !slices.Contains(p.UserTypes[name], client) {
		return errtypes.PermissionDenied(fmt.Sprintf("%s accounts cannot create app passwords for %s", name, client))
	}
	return nil
}

// expiry applies the default expiry and checks the maximum. Only sync clients,
// enrolled through the login flow, may have none.
func (m *mgr) expiry(client string, expiration *typespb.Timestamp) (*time.Time, error) {
	now := time.Now()
	t := toTime(expiration)
	switch {
	case t == nil && client == scope.ClientSync:
		return nil, nil
	case t == nil:
		d := now.AddDate(0, 0, m.c.DefaultExpiryDays)
		return &d, nil
	case !t.After(now):
		return nil, errtypes.BadRequest("the expiration is in the past")
	case t.After(now.AddDate(0, 0, m.c.MaxExpiryDays)):
		return nil, errtypes.BadRequest(fmt.Sprintf("the expiration is more than %d days away", m.c.MaxExpiryDays))
	}
	return t, nil
}

func (m *mgr) ListAppPasswords(ctx context.Context) ([]*apppb.AppPassword, error) {
	userID := appctx.ContextMustGetUser(ctx).GetId()

	var rows []model.AppPassword
	if err := m.db.WithContext(ctx).Where("user_id = ?", userID.GetOpaqueId()).Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]*apppb.AppPassword, 0, len(rows))
	for i := range rows {
		pw, err := m.appPassword(ctx, &rows[i], userID)
		if err != nil {
			return nil, err
		}
		out = append(out, pw)
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

// GetAppPassword looks an app password up by its secret. Without a user, the
// secret alone identifies it, and the result names its owner.
func (m *mgr) GetAppPassword(ctx context.Context, userID *userpb.UserId, secret string) (*apppb.AppPassword, error) {
	hash := hashToken(secret)

	q := m.db.WithContext(ctx).Where("token_hash = ?", hash)
	if userID != nil {
		q = q.Where("user_id = ?", userID.GetOpaqueId())
	}
	var row model.AppPassword
	err := q.First(&row).Error
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

	pw, err := m.appPassword(ctx, &row, userID)
	if err != nil {
		return nil, err
	}
	pw.Utime = &typespb.Timestamp{Seconds: uint64(now.Unix())} // report accurate last-seen
	return pw, nil
}

// appPassword converts a row, migrating it first if an older node wrote it.
func (m *mgr) appPassword(ctx context.Context, row *model.AppPassword, userID *userpb.UserId) (*apppb.AppPassword, error) {
	if row.PasswordID == "" {
		if err := migrateRow(ctx, m.db, row); err != nil {
			return nil, err
		}
	}
	return toAppPassword(row, userID)
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

// toAppPassword builds the scope of the app password from its columns.
func toAppPassword(row *model.AppPassword, userID *userpb.UserId) (*apppb.AppPassword, error) {
	if userID == nil {
		userID = &userpb.UserId{OpaqueId: row.UserID}
	}
	ap := &scope.AppPasswordScope{
		PasswordID:  row.PasswordID,
		Client:      row.Client,
		Permissions: row.Permissions,
		Roots:       row.Roots,
	}
	if row.Expiration != nil {
		ap.ExpiresAt = row.Expiration.Unix()
	}
	scopes, err := scope.AddAppPasswordScope(ap, nil)
	if err != nil {
		return nil, err
	}
	pw := &apppb.AppPassword{
		Password:   hex.EncodeToString(row.TokenHash), // stable handle, not the secret
		TokenScope: scopes,
		Label:      row.Label,
		Ctime:      &typespb.Timestamp{Seconds: uint64(row.Ctime.Unix())},
		Utime:      &typespb.Timestamp{Seconds: uint64(row.Utime.Unix())},
		User:       userID,
	}
	if row.Expiration != nil {
		pw.Expiration = &typespb.Timestamp{Seconds: uint64(row.Expiration.Unix())}
	}
	return pw, nil
}

func toTime(ts *typespb.Timestamp) *time.Time {
	if ts == nil || ts.Seconds == 0 {
		return nil
	}
	t := time.Unix(int64(ts.Seconds), int64(ts.Nanos)).UTC()
	return &t
}
