// Copyright 2018-2020 CERN
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

package local

import (
	"context"
	"database/sql"
	"path"

	"github.com/pkg/errors"

	// Provides sqlite drivers
	_ "github.com/mattn/go-sqlite3"
)

func initializeDB(root string) (*sql.DB, error) {
	dbPath := path.Join(root, "localfs.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error opening DB connection")
	}

	stmt, err := db.Prepare("CREATE TABLE IF NOT EXISTS recycled_entries (key TEXT PRIMARY KEY, path TEXT)")
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec()
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error executing create statement")
	}

	stmt, err = db.Prepare("CREATE TABLE IF NOT EXISTS user_interaction (resource TEXT, grantee TEXT, role TEXT DEFAULT '', favorite INTEGER DEFAULT 0) PRIMARY KEY (resource, grantee)")
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec()
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error executing create statement")
	}

	stmt, err = db.Prepare("CREATE TABLE IF NOT EXISTS metadata (resource TEXT PRIMARY KEY, mtime TEXT DEFAULT '', atime TEXT DEFAULT '', etag TEXT DEFAULT '')")
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec()
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error executing create statement")
	}

	return db, nil
}

func (fs *localfs) addToRecycledDB(ctx context.Context, key, fileName string) error {
	stmt, err := fs.db.Prepare("INSERT INTO recycled_entries VALUES (?, ?)")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(key, fileName)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing insert statement")
	}
	return nil
}

func (fs *localfs) getRecycledEntry(ctx context.Context, key string, path *string) error {
	return fs.db.QueryRow("SELECT path FROM recycled_entries WHERE key=?", key).Scan(*path)
}

func (fs *localfs) removeFromRecycledDB(ctx context.Context, key string) error {
	stmt, err := fs.db.Prepare("DELETE FROM recycled_entries WHERE key=?")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(key)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing delete statement")
	}
	return nil
}

func (fs *localfs) addToACLDB(ctx context.Context, resource, grantee, role string) error {
	stmt, err := fs.db.Prepare("INSERT INTO user_interaction (resource, grantee, role) VALUES (?, ?, ?) ON CONFLICT(resource, grantee) DO UPDATE SET role=?")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(resource, grantee, role, role)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing insert statement")
	}
	return nil
}

func (fs *localfs) getACLs(ctx context.Context, resource string) (*sql.Rows, error) {
	grants, err := fs.db.Query("SELECT grantee, role FROM user_interaction WHERE resource=?", resource)
	if err != nil {
		return nil, err
	}
	return grants, nil
}

func (fs *localfs) removeFromACLDB(ctx context.Context, resource, grantee string) error {
	stmt, err := fs.db.Prepare("UPDATE user_interaction SET role='' WHERE resource=? AND grantee=?")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(resource, grantee)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing delete statement")
	}
	return nil
}

func (fs *localfs) addToFavoritesDB(ctx context.Context, resource, grantee string) error {
	stmt, err := fs.db.Prepare("INSERT INTO user_interaction (resource, grantee, favorite) VALUES (?, ?, 1) ON CONFLICT(resource, grantee) DO UPDATE SET favorite=1")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(resource, grantee)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing insert statement")
	}
	return nil
}

func (fs *localfs) removeFromFavoritesDB(ctx context.Context, resource, grantee string) error {
	stmt, err := fs.db.Prepare("UPDATE user_interaction SET favorite=0 WHERE resource=? AND grantee=?")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(resource, grantee)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing delete statement")
	}
	return nil
}

func (fs *localfs) addToMtimeDB(ctx context.Context, resource, mtime, atime string) error {
	stmt, err := fs.db.Prepare("INSERT INTO metadata (resource, mtime, atime) VALUES (?, ?, ?) ON CONFLICT(resource) DO UPDATE SET mtime=?, atime=?")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(resource, mtime, atime, mtime, atime)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing insert statement")
	}
	return nil
}

func (fs *localfs) addToEtagDB(ctx context.Context, resource, etag string) error {
	stmt, err := fs.db.Prepare("INSERT INTO metadata (resource, etag) VALUES (?, ?) ON CONFLICT(resource) DO UPDATE SET etag=?")
	if err != nil {
		return errors.Wrap(err, "localfs: error preparing statement")
	}
	_, err = stmt.Exec(resource, etag, etag)
	if err != nil {
		return errors.Wrap(err, "localfs: error executing insert statement")
	}
	return nil
}
