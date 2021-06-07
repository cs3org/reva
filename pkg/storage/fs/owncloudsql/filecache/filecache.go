// Copyright 2018-2021 CERN
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

package filecache

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	conversions "github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

// Cache represents a oc10-style file cache
type Cache struct {
	driver string
	db     *sql.DB
}

// NewMysql returns a new Cache instance connecting to a MySQL database
func NewMysql(dsn string) (*Cache, error) {
	sqldb, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to the database")
	}
	sqldb.SetConnMaxLifetime(time.Minute * 3)
	sqldb.SetMaxOpenConns(10)
	sqldb.SetMaxIdleConns(10)

	err = sqldb.Ping()
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to the database")
	}

	return New("mysql", sqldb)
}

// New returns a new Cache instance connecting to the given sql.DB
func New(driver string, sqldb *sql.DB) (*Cache, error) {
	return &Cache{
		driver: driver,
		db:     sqldb,
	}, nil
}

// GetNumericStorageID returns the database id for the given storage
func (c *Cache) GetNumericStorageID(id string) (int, error) {
	row := c.db.QueryRow("Select numeric_id from oc_storages where id = ?", id)
	var nid int
	switch err := row.Scan(&nid); err {
	case nil:
		return nid, nil
	default:
		return -1, err
	}
}

// File represents an entry of the file cache
type File struct {
	ID              int
	Storage         int
	Parent          int
	MimePart        int
	MimeType        int
	Size            int
	MTime           int
	StorageMTime    int
	UnencryptedSize int
	Permissions     int
	Encrypted       bool
	Path            string
	Name            string
	Etag            string
	Checksum        string
}

// TrashItem represents a trash item of the file cache
type TrashItem struct {
	ID        int
	Name      string
	User      string
	Path      string
	Timestamp int
}

// Scannable describes the interface providing a Scan method
type Scannable interface {
	Scan(...interface{}) error
}

func (c *Cache) rowToFile(row Scannable) (*File, error) {
	var fileid, storage, parent, mimetype, mimepart, size, mtime, storageMtime, encrypted, unencryptedSize, permissions int
	var path, name, etag, checksum string
	err := row.Scan(&fileid, &storage, &path, &parent, &permissions, &mimetype, &mimepart, &size, &mtime, &storageMtime, &encrypted, &unencryptedSize, &name, &etag, &checksum)
	if err != nil {
		return nil, err
	}

	return &File{
		ID:              fileid,
		Storage:         storage,
		Path:            path,
		Parent:          parent,
		Permissions:     permissions,
		MimeType:        mimetype,
		MimePart:        mimepart,
		Size:            size,
		MTime:           mtime,
		StorageMTime:    storageMtime,
		Encrypted:       encrypted == 1,
		UnencryptedSize: unencryptedSize,
		Name:            name,
		Etag:            etag,
		Checksum:        checksum,
	}, nil
}

// Get returns the cache entry for the specified storage/path
func (c *Cache) Get(s interface{}, p string) (*File, error) {
	storageID, err := toIntID(s)
	if err != nil {
		return nil, err
	}

	phashBytes := md5.Sum([]byte(p))
	phash := hex.EncodeToString(phashBytes[:])

	row := c.db.QueryRow("Select fileid, storage, path, parent, permissions, mimetype, mimepart, size, mtime, storage_mtime, encrypted, unencrypted_size, name, etag, checksum from oc_filecache where path_hash = ? and storage = ?", phash, storageID)
	return c.rowToFile(row)
}

// Path returns the path for the specified entry
func (c *Cache) Path(id interface{}) (string, error) {
	id, err := toIntID(id)
	if err != nil {
		return "", err
	}

	row := c.db.QueryRow("Select path from oc_filecache where fileid = ?", id)
	var path string
	err = row.Scan(&path)
	if err != nil {
		return "", err
	}
	return path, nil
}

// Permissions returns the permissions for the specified storage/path
func (c *Cache) Permissions(storage interface{}, p string) (*provider.ResourcePermissions, error) {
	entry, err := c.Get(storage, p)
	if err != nil {
		return nil, err
	}

	perms, err := conversions.NewPermissions(entry.Permissions)
	if err != nil {
		return nil, err
	}

	return conversions.RoleFromOCSPermissions(perms).CS3ResourcePermissions(), nil
}

// InsertOrUpdate creates or updates a cache entry
func (c *Cache) InsertOrUpdate(storage interface{}, data map[string]interface{}) (int, error) {
	storageID, err := toIntID(storage)
	if err != nil {
		return -1, err
	}

	columns := []string{"storage"}
	placeholders := []string{"?"}
	values := []interface{}{storage}

	for _, key := range []string{"path", "mimetype", "etag"} {
		if _, exists := data[key]; !exists {
			return -1, fmt.Errorf("missing required data")
		}
	}

	path := data["path"].(string)
	parentPath := strings.TrimRight(filepath.Dir(path), "/")
	if parentPath == "." {
		parentPath = ""
	}
	parent, err := c.Get(storageID, parentPath)
	if err != nil {
		return -1, fmt.Errorf("could not find parent %s, %s, %v, %w", parentPath, path, parent, err)
	}
	data["parent"] = parent.ID
	data["name"] = filepath.Base(path)
	if _, exists := data["checksum"]; !exists {
		data["checksum"] = ""
	}

	for k, v := range data {
		switch k {
		case "path":
			phashBytes := md5.Sum([]byte(v.(string)))
			phash := hex.EncodeToString(phashBytes[:])
			columns = append(columns, "path_hash")
			values = append(values, phash)
			placeholders = append(placeholders, "?")
		case "storage_mtime":
			if _, exists := data["mtime"]; !exists {
				columns = append(columns, "mtime")
				values = append(values, v)
				placeholders = append(placeholders, "?")
			}
		case "mimetype":
			parts := strings.Split(v.(string), "/")
			columns = append(columns, "mimetype")
			values = append(values, v)
			placeholders = append(placeholders, "(SELECT id from oc_mimetypes where mimetype=?)")
			columns = append(columns, "mimepart")
			values = append(values, parts[0])
			placeholders = append(placeholders, "(SELECT id from oc_mimetypes where mimetype=?)")
			continue
		}

		columns = append(columns, k)
		values = append(values, v)
		placeholders = append(placeholders, "?")
	}

	err = c.InsertMimetype(data["mimetype"].(string))
	if err != nil {
		return -1, err
	}

	query := "INSERT INTO oc_filecache( " + strings.Join(columns, ", ") + ") VALUES(" + strings.Join(placeholders, ",") + ")"

	updates := []string{}
	for i, column := range columns {
		if column != "path" && column != "path_hash" && column != "storage" {
			updates = append(updates, column+"="+placeholders[i])
			values = append(values, values[i])
		}
	}
	if c.driver == "mysql" { // mysql upsert
		query += " ON DUPLICATE KEY UPDATE "
	} else { // sqlite3 upsert
		query += " ON CONFLICT(storage,path_hash) DO UPDATE SET "
	}
	query += strings.Join(updates, ",")

	stmt, err := c.db.Prepare(query)
	if err != nil {
		return -1, err
	}

	res, err := stmt.Exec(values...)
	if err != nil {
		log.Err(err).Msg("could not store filecache item")
		return -1, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return -1, err
	}
	return int(id), nil
}

// Copy creates a copy of the specified entry at the target path
func (c *Cache) Copy(storage interface{}, sourcePath, targetPath string) (int, error) {
	storageID, err := toIntID(storage)
	if err != nil {
		return -1, err
	}
	source, err := c.Get(storageID, sourcePath)
	if err != nil {
		return -1, errors.Wrap(err, "could not find source")
	}

	row := c.db.QueryRow("SELECT mimetype FROM oc_mimetypes WHERE id=?", source.MimeType)
	var mimetype string
	err = row.Scan(&mimetype)
	if err != nil {
		return -1, errors.Wrap(err, "could not find source mimetype")
	}

	data := map[string]interface{}{
		"path":             targetPath,
		"checksum":         source.Checksum,
		"mimetype":         mimetype,
		"permissions":      source.Permissions,
		"etag":             source.Etag,
		"size":             source.Size,
		"mtime":            source.MTime,
		"storage_mtime":    source.StorageMTime,
		"encrypted":        source.Encrypted,
		"unencrypted_size": source.UnencryptedSize,
	}
	return c.InsertOrUpdate(storage, data)
}

// Move moves the specified entry to the target path
func (c *Cache) Move(storage interface{}, sourcePath, targetPath string) error {
	storageID, err := toIntID(storage)
	if err != nil {
		return err
	}
	source, err := c.Get(storageID, sourcePath)
	if err != nil {
		return errors.Wrap(err, "could not find source")
	}
	newParentPath := strings.TrimRight(filepath.Dir(targetPath), "/")
	newParent, err := c.Get(storageID, newParentPath)
	if err != nil {
		return errors.Wrap(err, "could not find new parent")
	}

	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare("UPDATE oc_filecache SET parent=?, path=?, name=?, path_hash=? WHERE storage = ? and fileid=?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	phashBytes := md5.Sum([]byte(targetPath))
	_, err = stmt.Exec(newParent.ID, targetPath, filepath.Base(targetPath), hex.EncodeToString(phashBytes[:]), storageID, source.ID)
	if err != nil {
		return err
	}

	childRows, err := tx.Query("SELECT fileid, path from oc_filecache where parent = ?", source.ID)
	if err != nil {
		return err
	}
	defer childRows.Close()
	children := map[int]string{}
	for childRows.Next() {
		var (
			id   int
			path string
		)
		err = childRows.Scan(&id, &path)
		if err != nil {
			return err
		}

		children[id] = path
	}
	for id, path := range children {
		path = strings.ReplaceAll(path, sourcePath, targetPath)
		phashBytes = md5.Sum([]byte(path))
		_, err = stmt.Exec(source.ID, path, filepath.Base(path), hex.EncodeToString(phashBytes[:]), storageID, id)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Delete removes the specified storage/path from the cache
func (c *Cache) Delete(storage interface{}, user, path, trashPath string) error {
	err := c.Move(storage, path, trashPath)
	if err != nil {
		return err
	}

	re := regexp.MustCompile(`(.*)\.d(\d+)$`)
	parts := re.FindStringSubmatch(filepath.Base(trashPath))

	query := "INSERT INTO oc_files_trash(user,id,timestamp,location) VALUES(?,?,?,?)"
	stmt, err := c.db.Prepare(query)
	if err != nil {
		return err
	}

	relativeLocation, err := filepath.Rel("files/", filepath.Dir(path))
	if err != nil {
		return err
	}
	_, err = stmt.Exec(user, filepath.Base(parts[1]), parts[2], relativeLocation)
	if err != nil {
		log.Err(err).Msg("could not store filecache item")
		return err
	}

	return nil
}

// GetRecycleItem returns the specified recycle item
func (c *Cache) GetRecycleItem(user, path string, timestamp int) (*TrashItem, error) {
	row := c.db.QueryRow("SELECT auto_id, id, location FROM oc_files_trash WHERE id = ? and user = ? and timestamp = ?", path, user, timestamp)
	var autoID int
	var id, location string
	err := row.Scan(&autoID, &id, &location)
	if err != nil {
		return nil, err
	}

	return &TrashItem{
		ID:        autoID,
		Name:      id,
		User:      user,
		Path:      location,
		Timestamp: timestamp,
	}, nil
}

// PurgeRecycleItem deletes the specified item from the cache
func (c *Cache) PurgeRecycleItem(user, path string, timestamp int) error {
	row := c.db.QueryRow("Select auto_id, location from oc_files_trash where id = ? and user = ? and timestamp = ?", path, user, timestamp)
	var autoID int
	var location string
	err := row.Scan(&autoID, &location)
	if err != nil {
		return err
	}

	_, err = c.db.Exec("DELETE FROM oc_files_trash WHERE auto_id=?", autoID)
	if err != nil {
		return err
	}

	storage, err := c.GetNumericStorageID("home::" + user)
	if err != nil {
		return err
	}
	item, err := c.Get(storage, filepath.Join("files_trashbin", "files", location, path+".d"+strconv.Itoa(timestamp)))
	if err != nil {
		return err
	}
	_, err = c.db.Exec("DELETE FROM oc_filecache WHERE fileid=? OR parent=?", item.ID, item.ID)

	return err
}

// SetEtag set a new etag for the specified item
func (c *Cache) SetEtag(storage interface{}, path, etag string) error {
	storageID, err := toIntID(storage)
	if err != nil {
		return err
	}
	source, err := c.Get(storageID, path)
	if err != nil {
		return errors.Wrap(err, "could not find source")
	}
	stmt, err := c.db.Prepare("UPDATE oc_filecache SET etag=? WHERE storage = ? and fileid=?")
	if err != nil {
		return err
	}
	_, err = stmt.Exec(etag, storageID, source.ID)
	return err
}

// InsertMimetype adds a new mimetype to the database
func (c *Cache) InsertMimetype(mimetype string) error {
	stmt, err := c.db.Prepare("INSERT INTO oc_mimetypes(mimetype) VALUES(?)")
	if err != nil {
		return err
	}
	_, err = stmt.Exec(mimetype)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "Error 1062") {
			return nil // Already exists
		}
		return err
	}
	return nil
}

func toIntID(rid interface{}) (int, error) {
	switch t := rid.(type) {
	case int:
		return t, nil
	case string:
		return strconv.Atoi(t)
	default:
		return -1, fmt.Errorf("invalid type")
	}
}
