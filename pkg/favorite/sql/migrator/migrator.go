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

package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/cs3org/reva/v3/pkg/favorite/sql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// OldFavorite represents the old database table structure for favorites
type OldFavorite struct {
	ID           uint    `gorm:"primarykey"`
	ItemType     int     `gorm:"column:item_type"`
	UID          string  `gorm:"column:uid"`
	FileidPrefix string  `gorm:"column:fileid_prefix"`
	Fileid       string  `gorm:"column:fileid"`
	TagKey       string  `gorm:"column:tag_key"`
	TagVal       *string `gorm:"column:tag_val"`
}

// MigrateOldFavoritesToNew migrates data from the old tags table to the new favorites table
func MigrateOldFavoritesToNew(db *gorm.DB) error {
	var oldFavs []OldFavorite

	// Assuming the old table is named "tags" and favorites are marked with tag_key = 'fav'
	err := db.Table("cbox_metadata").Where("tag_key = ?", "fav").Find(&oldFavs).Error
	if err != nil {
		return err
	}

	log.Printf("Found %d old favorites to migrate", len(oldFavs))

	for _, old := range oldFavs {
		fav := sql.Favorite{
			UserId:   old.UID,
			Inode:    old.Fileid,
			Instance: old.FileidPrefix,
		}

		err = db.Where(sql.Favorite{UserId: fav.UserId, Inode: fav.Inode, Instance: fav.Instance}).Attrs(fav).FirstOrCreate(&fav).Error
		if err != nil {
			return err
		}
	}

	log.Printf("Successfully migrated %d favorites", len(oldFavs))
	return nil
}

func main() {
	var dbName = flag.String("dbname", "test", "Database name")
	var dbHost = flag.String("host", "dbod-cboxeos.cern.ch", "Database host")
	var dbPort = flag.Int("port", 5504, "Database port")
	var dbUsername = flag.String("username", "cerbox_server", "Database username")
	var dbPassword = flag.String("password", "", "Database password")
	flag.Parse()

	var db *gorm.DB
	var err error

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", *dbUsername, *dbPassword, *dbHost, *dbPort, *dbName)
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})

	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	err = MigrateOldFavoritesToNew(db)
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	fmt.Println("Migration completed successfully")
}
