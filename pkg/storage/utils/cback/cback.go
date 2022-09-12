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

package cback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/pkg/errors"
)

// Config is the config used by the cback client
type Config struct {
	URL      string
	Token    string
	Insecure bool
	Timeout  int
}

// Client is the client to connect to cback
type Client struct {
	c      *Config
	client *http.Client
}

// New creates a new cback client
func New(c *Config) *Client {
	return &Client{
		c: c,
		client: rhttp.GetHTTPClient(
			rhttp.Insecure(c.Insecure),
			rhttp.Timeout(time.Duration(c.Timeout)),
		),
	}
}

func (c *Client) doHTTPRequest(ctx context.Context, username, reqType, endpoint string, body io.Reader) (io.ReadCloser, error) {
	url := c.c.URL + endpoint
	req, err := http.NewRequestWithContext(ctx, reqType, url, body)
	if err != nil {
		return nil, errors.Wrapf(err, "error creationg http %s request to %s", reqType, url)
	}

	req.SetBasicAuth(username, c.c.Token)

	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}

	req.Header.Add("accept", `application/json`)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, errtypes.NotFound("cback: resource not found")
		case http.StatusForbidden:
			return nil, errtypes.PermissionDenied("cback: user has no permissions to get the backup")
		case http.StatusBadRequest:
			return nil, errtypes.BadRequest("")
		default:
			return nil, errtypes.InternalError("cback: internal server error: " + resp.Status)
		}
	}

	return resp.Body, nil
}

// ListBackups gets all the backups of a user
func (c *Client) ListBackups(ctx context.Context, username string) ([]*Backup, error) {
	body, err := c.doHTTPRequest(ctx, username, http.MethodGet, "/backups/", nil)
	if err != nil {
		return nil, errors.Wrap(err, "cback: error listing backups for user "+username)
	}
	defer body.Close()

	var backups []*Backup

	if err := json.NewDecoder(body).Decode(&backups); err != nil {
		return nil, errors.Wrap(err, "cback: error decoding response body for backups' list")
	}

	return backups, nil
}

// ListSnapshots gets all the snapshots of a backup
func (c *Client) ListSnapshots(ctx context.Context, username string, backupID int) ([]*Snapshot, error) {
	endpoint := fmt.Sprintf("/backups/%d/snapshots", backupID)
	body, err := c.doHTTPRequest(ctx, username, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "cback: error listing snapshots for backup %d", backupID)
	}
	defer body.Close()

	var snapshots []*Snapshot

	if err := json.NewDecoder(body).Decode(&snapshots); err != nil {
		return nil, errors.Wrap(err, "cbacK: error decoding response body for snapshots' list")
	}

	return snapshots, nil
}

// Stat gets the info of a resource stored in cback
func (c *Client) Stat(ctx context.Context, username string, backupID int, snapshotID, path string, isTimestamp bool) (*Resource, error) {
	endpoint := fmt.Sprintf("/backups/%d/snapshots/%s/%s", backupID, snapshotID, path)
	if isTimestamp {
		endpoint += "?timestamp=true"
	}
	body, err := c.doHTTPRequest(ctx, username, http.MethodOptions, endpoint, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "cback: error statting %s in snapshot %s in backup %d", path, snapshotID, backupID)
	}
	defer body.Close()

	var res *Resource

	if err := json.NewDecoder(body).Decode(&res); err != nil {
		return nil, errors.Wrap(err, "cback: error decoding response body")
	}

	return res, nil
}

// ListFolder gets the content of a folder stored in cback
func (c *Client) ListFolder(ctx context.Context, username string, backupID int, snapshotID, path string, isTimestamp bool) ([]*Resource, error) {
	endpoint := fmt.Sprintf("/backups/%d/snapshots/%s/%s?content=true", backupID, snapshotID, path)
	if isTimestamp {
		endpoint += "&timestamp=true"
	}
	body, err := c.doHTTPRequest(ctx, username, http.MethodOptions, endpoint, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "cback: error statting %s in snapshot %s in backup %d", path, snapshotID, backupID)
	}
	defer body.Close()

	var res []*Resource

	if err := json.NewDecoder(body).Decode(&res); err != nil {
		return nil, errors.Wrap(err, "cback: error decoding response body")
	}

	return res, nil
}

// Download gets the content of a file stored in cback
func (c *Client) Download(ctx context.Context, username string, backupID int, snapshotID, path string, isTimestamp bool) (io.ReadCloser, error) {
	endpoint := fmt.Sprintf("/backups/%d/snapshots/%s/%s", backupID, snapshotID, path)
	if isTimestamp {
		endpoint += "?timestamp=true"
	}
	return c.doHTTPRequest(ctx, username, http.MethodGet, endpoint, nil)
}

// ListRestores gets the list of restore jobs created by the user
func (c *Client) ListRestores(ctx context.Context, username string) ([]*Restore, error) {
	body, err := c.doHTTPRequest(ctx, username, http.MethodGet, "/restores/", nil)
	if err != nil {
		return nil, errors.Wrap(err, "cback: error getting restores")
	}
	defer body.Close()

	var res []*Restore

	if err := json.NewDecoder(body).Decode(&res); err != nil {
		return nil, errors.Wrap(err, "cback: error decoding response body")
	}

	return res, nil
}

// GetRestore get the info of a restore job
func (c *Client) GetRestore(ctx context.Context, username string, restoreID int) (*Restore, error) {
	endpoint := fmt.Sprintf("/restores/%d", restoreID)
	body, err := c.doHTTPRequest(ctx, username, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "cback: error getting restores")
	}
	defer body.Close()

	var res *Restore

	if err := json.NewDecoder(body).Decode(&res); err != nil {
		return nil, errors.Wrap(err, "cback: error decoding response body")
	}

	return res, nil
}

type newRestoreRequest struct {
	BackupID int    `json:"backup_id"`
	Pattern  string `json:"pattern"`
	Snapshot string `json:"snapshot"`
}

// NewRestore creates a new restore job in cback
func (c *Client) NewRestore(ctx context.Context, username string, backupID int, pattern, snapshotID string) (*Restore, error) {
	req, err := json.Marshal(newRestoreRequest{
		BackupID: backupID,
		Pattern:  pattern,
		Snapshot: snapshotID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "cback: error marshaling new restore request")
	}

	body, err := c.doHTTPRequest(ctx, username, http.MethodPost, "/restores/", bytes.NewReader(req))
	if err != nil {
		return nil, errors.Wrap(err, "cback: error getting restores")
	}
	defer body.Close()

	var res *Restore

	if err := json.NewDecoder(body).Decode(&res); err != nil {
		return nil, errors.Wrap(err, "cback: error decoding response body")
	}

	return res, nil
}
