package sql

import (
	"fmt"

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	ocmshareregistry "github.com/cs3org/reva/v3/pkg/ocm/share/repository/registry"
	publicshareregistry "github.com/cs3org/reva/v3/pkg/publicshare/manager/registry"
	shareregistry "github.com/cs3org/reva/v3/pkg/share/manager/registry"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	projectInstancesPrefix        = "newproject"
	projectSpaceGroupsPrefix      = "cernbox-project-"
	projectSpaceAdminGroupsSuffix = "-admins"
	projectPathPrefix             = "/eos/project/"
)

type Config struct {
	config.Database      `mapstructure:",squash"`
	GatewaySvc           string `mapstructure:"gatewaysvc"`
	LinkPasswordHashCost int    `mapstructure:"password_hash_cost"`
}

func init() {
	shareregistry.Register("sql", NewShareManager)
	publicshareregistry.Register("sql", NewPublicShareManager)
	ocmshareregistry.Register("sql", NewOCMShareManager)
}

func (c *Config) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
	c.Database = sharedconf.GetDBInfo(c.Database)
}

func getDb(c Config) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: false,
	}
	switch c.Engine {
	case "sqlite":
		return gorm.Open(sqlite.Open(c.DBName), gormCfg)
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		return gorm.Open(mysql.Open(dsn), gormCfg)
	default: // default is mysql
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		return gorm.Open(mysql.Open(dsn), gormCfg)
	}
}

func createID(db *gorm.DB) (uint, error) {
	id := &model.ShareID{}

	res := db.Create(&id)
	if res.Error != nil {
		return 0, res.Error
	} else {
		return id.ID, nil
	}
}
