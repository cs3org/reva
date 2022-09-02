package cback

import (
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

type Config struct {
	URL      string
	Token    string
	Insecure bool
	Timeout  int
}

type Client struct {
	c      *Config
	client *http.Client
}

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

func (c *Client) Stat(ctx context.Context, username string, backupID int, snapshotID, path string) (*Resource, error) {
	endpoint := fmt.Sprintf("/backups/%d/snapshots/%s/%s", backupID, snapshotID, path)
	body, err := c.doHTTPRequest(ctx, username, http.MethodOptions, endpoint, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "cback: error statting %s in snapshot %s in backup %d", path, snapshotID, backupID)
	}
	defer body.Close()

	var res *Resource

	if err := json.NewDecoder(body).Decode(res); err != nil {
		return nil, errors.Wrap(err, "cback: error decoding response body")
	}

	return res, nil
}

func (c *Client) ListFolder(ctx context.Context, username string, backupID int, snapshotID, path string) ([]*Resource, error) {
	endpoint := fmt.Sprintf("/backups/%d/snapshots/%s/%s?content=true", backupID, snapshotID, path)
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

func (c *Client) Download(ctx context.Context, username string, backupID int, snapshotsID, path string) (io.ReadCloser, error) {
	endpoint := fmt.Sprintf("/backups/%d/snapshots/%s/%s", backupID, snapshotsID, path)
	return c.doHTTPRequest(ctx, username, http.MethodGet, endpoint, nil)
}
