package sql

import (
	"context"
	"fmt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/user/outgoing"
	"github.com/cs3org/reva/v3/pkg/user/outgoing/manager/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	registry.Register("sql", New)
}

// Config is the configuration to use for the sql driver
// implementing the outgoing.Manager interface.
type Config struct {
	config.Database `mapstructure:",squash"`
}

type OutgoingUserManager struct {
	c  *Config
	db *gorm.DB
}

// OutgoingUser represents an outgoing user in the DB.
type OutgoingUser struct {
	gorm.Model
	// User identifier (opaque user ID)
	User string `gorm:"size:255;uniqueIndex:idx_user;not null"`
	// Status of the outgoing user (graceperiod or archiving)
	Status outgoing.OutgoingUserStatus `gorm:"size:50;index:idx_status;not null"`
}

func New(ctx context.Context, m map[string]any) (outgoing.Manager, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	c.ApplyDefaults()

	var db *gorm.DB
	var err error
	switch c.Engine {
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(c.DBName), &gorm.Config{})
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	default: // default is mysql
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	}
	if err != nil {
		return nil, errors.Wrap(err, "Failed to connect to OutgoingUsers database using engine "+c.Engine)
	}

	// Migrate schemas
	err = db.AutoMigrate(&OutgoingUser{})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to migrate OutgoingUser schema")
	}

	mgr := &OutgoingUserManager{
		c:  &c,
		db: db,
	}

	return mgr, nil
}

func (m *OutgoingUserManager) AddOutgoingUser(ctx context.Context, user *userpb.UserId, status outgoing.OutgoingUserStatus) error {
	outgoingUser := &OutgoingUser{
		User:   user.OpaqueId,
		Status: status,
	}

	result := m.db.Create(outgoingUser)
	if result.Error != nil {
		return errors.Wrap(result.Error, "Failed to add outgoing user")
	}

	return nil
}

func (m *OutgoingUserManager) GetOutgoingUser(ctx context.Context, user *userpb.UserId) (outgoing.OutgoingUserStatus, error) {
	var outgoingUser OutgoingUser
	result := m.db.Where("user = ?", user.OpaqueId).First(&outgoingUser)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return "", errors.New("Outgoing user not found")
		}
		return "", errors.Wrap(result.Error, "Failed to get outgoing user")
	}

	return outgoingUser.Status, nil
}

func (m *OutgoingUserManager) UpdateOutgoingUserStatus(ctx context.Context, user *userpb.UserId, status outgoing.OutgoingUserStatus) error {
	result := m.db.Model(&OutgoingUser{}).
		Where("user = ?", user.OpaqueId).
		Update("status", status)

	if result.Error != nil {
		return errors.Wrap(result.Error, "Failed to update outgoing user status")
	}

	if result.RowsAffected == 0 {
		return errors.New("Outgoing user not found")
	}

	return nil
}

func (m *OutgoingUserManager) RemoveOutgoingUser(ctx context.Context, user *userpb.UserId) error {
	result := m.db.Unscoped().Where("user = ?", user.OpaqueId).Delete(&OutgoingUser{})
	if result.Error != nil {
		return errors.Wrap(result.Error, "Failed to remove outgoing user")
	}

	if result.RowsAffected == 0 {
		return errors.New("Outgoing user not found")
	}

	return nil
}

func (m *OutgoingUserManager) ListOutgoingUsers(ctx context.Context, status *outgoing.OutgoingUserStatus) ([]outgoing.OutgoingUserInfo, error) {
	var outgoingUsers []OutgoingUser
	query := m.db

	if status != nil {
		query = query.Where("status = ?", *status)
	}

	result := query.Find(&outgoingUsers)
	if result.Error != nil {
		return nil, errors.Wrap(result.Error, "Failed to list outgoing users")
	}

	userInfos := make([]outgoing.OutgoingUserInfo, len(outgoingUsers))
	for i, ou := range outgoingUsers {
		userInfos[i] = outgoing.OutgoingUserInfo{
			User: &userpb.UserId{
				OpaqueId: ou.User,
			},
			Status: ou.Status,
		}
	}

	return userInfos, nil
}

func (c *Config) ApplyDefaults() {
	c.Database = sharedconf.GetDBInfo(c.Database)
}
