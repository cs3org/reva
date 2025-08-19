//go:build ceph

// Copyright 2018-2024 CERN
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

// GetPathByID implementation with ceph support using go-ceph library
package nceph

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	goceph "github.com/ceph/go-ceph/cephfs"
	rados2 "github.com/ceph/go-ceph/rados"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/pkg/errors"
)

// CephAdminConn represents the admin connection to ceph for GetPathByID operations
type CephAdminConn struct {
	radosConn *rados2.Conn
	fsMount   *goceph.MountInfo
}

// newCephAdminConn creates a ceph admin connection for GetPathByID operations
func newCephAdminConn(ctx context.Context, conf *Options) (*CephAdminConn, error) {
	log := appctx.GetLogger(ctx)

	// Validate configuration
	if conf.CephConfig == "" || conf.CephClientID == "" || conf.CephKeyring == "" {
		return nil, errors.New("nceph: incomplete ceph configuration")
	}

	// Create rados connection
	conn, err := rados2.NewConnWithUser(conf.CephClientID)
	if err != nil {
		return nil, errors.Wrap(err, "nceph: failed to create rados connection")
	}

	// Set configuration
	err = conn.ReadConfigFile(conf.CephConfig)
	if err != nil {
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to read ceph config file")
	}

	// Set keyring
	err = conn.SetConfigOption("keyring", conf.CephKeyring)
	if err != nil {
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to set keyring")
	}

	// Connect to cluster
	err = conn.Connect()
	if err != nil {
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to connect to ceph cluster")
	}

	// Create cephfs mount
	fsMount, err := goceph.CreateMountWithId(conf.CephClientID)
	if err != nil {
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to create cephfs mount")
	}

	// Configure mount with the same configuration
	err = fsMount.ReadConfigFile(conf.CephConfig)
	if err != nil {
		fsMount.Release()
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to read config for mount")
	}

	// Set keyring for mount
	err = fsMount.SetConfigOption("keyring", conf.CephKeyring)
	if err != nil {
		fsMount.Release()
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to set keyring for mount")
	}

	// Mount the filesystem
	err = fsMount.Mount()
	if err != nil {
		fsMount.Release()
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to mount cephfs")
	}

	log.Info().Msg("Successfully created ceph admin connection for GetPathByID with go-ceph library")

	return &CephAdminConn{
		radosConn: conn,
		fsMount:   fsMount,
	}, nil
}

// Close closes the ceph admin connection
func (c *CephAdminConn) Close() {
	if c.fsMount != nil {
		c.fsMount.Unmount()
		c.fsMount.Release()
	}
	if c.radosConn != nil {
		c.radosConn.Shutdown()
	}
}

func (fs *ncephfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	if fs.cephAdminConn == nil {
		return "", errtypes.NotSupported("nceph: GetPathByID requires ceph configuration")
	}

	log := appctx.GetLogger(ctx)
	log.Debug().Str("resourceId", id.OpaqueId).Msg("GetPathByID with advanced ceph support")

	// Convert resource ID to inode number
	inode, err := strconv.ParseInt(id.OpaqueId, 10, 64)
	if err != nil {
		return "", errors.Wrap(err, "nceph: invalid resource ID format")
	}

	// Get path by inode using cephfs mount
	path, err := fs.getPathByInode(ctx, inode)
	if err != nil {
		return "", errors.Wrap(err, "nceph: failed to get path by inode")
	}

	// Remove ceph root prefix if configured
	if fs.conf.CephRoot != "" {
		path = strings.TrimPrefix(path, fs.conf.CephRoot)
	}

	// Ensure path starts with /
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	log.Debug().Str("path", path).Int64("inode", inode).Msg("Successfully resolved path by ID")
	return path, nil
}

// getPathByInode uses cephfs API to resolve inode to path
func (fs *ncephfs) getPathByInode(ctx context.Context, inode int64) (string, error) {
	log := appctx.GetLogger(ctx)

	// Method 1: Try using MgrCommand to get filesystem status and find active MDS
	mdsSpec, err := fs.getActiveMDS(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get active MDS, trying direct approach")
	} else {
		// Method 2: Use MdsCommand to dump inode information
		path, err := fs.dumpInodeViaCommand(ctx, mdsSpec, inode)
		if err == nil {
			return path, nil
		}
		log.Warn().Err(err).Msg("Failed to dump inode via command, trying mount API")
	}

	// Method 3: Try using the cephfs mount API directly
	// This is a fallback method that doesn't require MDS commands
	return "", errtypes.NotSupported("nceph: inode to path resolution requires MDS admin access")
}

// getActiveMDS gets the active MDS using manager commands
func (fs *ncephfs) getActiveMDS(ctx context.Context) (string, error) {
	// Prepare fs status command
	cmd, err := json.Marshal(map[string]interface{}{
		"prefix": "fs status",
		"format": "json",
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal fs status command")
	}

	// Execute manager command
	buf, info, err := fs.cephAdminConn.radosConn.MgrCommand([][]byte{cmd})
	if err != nil {
		return "", errors.Wrap(err, "failed to execute fs status command")
	}

	if info != "" {
		log := appctx.GetLogger(ctx)
		log.Debug().Str("info", info).Msg("Manager command info")
	}

	// Parse response to find active MDS
	var fsStatus struct {
		MDSMap struct {
			Info map[string]struct {
				Name  string `json:"name"`
				State string `json:"state"`
			} `json:"info"`
		} `json:"mdsmap"`
	}

	if err := json.Unmarshal(buf, &fsStatus); err != nil {
		return "", errors.Wrap(err, "failed to parse fs status response")
	}

	// Find active MDS
	for _, mds := range fsStatus.MDSMap.Info {
		if strings.Contains(mds.State, "active") {
			return mds.Name, nil
		}
	}

	return "", errors.New("no active MDS found")
}

// dumpInodeViaCommand uses MDS commands to dump inode information
func (fs *ncephfs) dumpInodeViaCommand(ctx context.Context, mdsName string, inode int64) (string, error) {
	// Prepare dump inode command
	cmd, err := json.Marshal(map[string]interface{}{
		"prefix": "mds metadata",
		"who":    mdsName,
		"format": "json",
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal mds metadata command")
	}

	// Execute MDS command
	buf, info, err := fs.cephAdminConn.radosConn.MgrCommand([][]byte{cmd})
	if err != nil {
		return "", errors.Wrap(err, "failed to execute mds metadata command")
	}

	if info != "" {
		log := appctx.GetLogger(ctx)
		log.Debug().Str("info", info).Msg("MDS command info")
	}

	// For a real implementation, you would need to:
	// 1. Get the MDS rank/address from the metadata
	// 2. Connect directly to the MDS
	// 3. Send an inode dump command
	// 4. Parse the response to extract the path

	// This is a simplified approach - in production you might use:
	// - Direct MDS socket connection
	// - Custom admin socket commands
	// - CephFS admin API calls

	log := appctx.GetLogger(ctx)
	log.Debug().
		Str("mds", mdsName).
		Int64("inode", inode).
		Str("buffer", string(buf)).
		Msg("Would dump inode information via MDS command")

	// For now, return a placeholder indicating the command structure is ready
	// but actual inode->path resolution requires more specific MDS integration
	return "", fmt.Errorf("inode %d path resolution via MDS '%s' not fully implemented", inode, mdsName)
}
