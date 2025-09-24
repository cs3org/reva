package sql

import (
	"fmt"

	publicshareregistry "github.com/cs3org/reva/v3/pkg/publicshare/manager/registry"
	shareregistry "github.com/cs3org/reva/v3/pkg/share/manager/registry"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
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

type config struct {
	Engine               string `mapstructure:"engine"` // mysql | sqlite
	DBUsername           string `mapstructure:"db_username"`
	DBPassword           string `mapstructure:"db_password"`
	DBHost               string `mapstructure:"db_host"`
	DBPort               int    `mapstructure:"db_port"`
	DBName               string `mapstructure:"db_name"`
	GatewaySvc           string `mapstructure:"gatewaysvc"`
	LinkPasswordHashCost int    `mapstructure:"password_hash_cost"`
}

func init() {
	shareregistry.Register("sql", NewShareManager)
	publicshareregistry.Register("sql", NewPublicShareManager)
}

func getDb(c config) (*gorm.DB, error) {
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
