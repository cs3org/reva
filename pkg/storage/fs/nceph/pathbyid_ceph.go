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
	"regexp"
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
	log.Debug().Msg("nceph: Starting ceph admin connection creation")

	// Validate configuration
	log.Debug().Str("ceph_config", conf.CephConfig).Str("client_id", conf.CephClientID).Str("keyring", conf.CephKeyring).Msg("nceph: Validating ceph configuration")
	if conf.CephConfig == "" || conf.CephClientID == "" || conf.CephKeyring == "" {
		log.Error().Str("ceph_config", conf.CephConfig).Str("client_id", conf.CephClientID).Str("keyring", conf.CephKeyring).Msg("nceph: Incomplete ceph configuration")
		return nil, errors.New("nceph: incomplete ceph configuration")
	}
	log.Debug().Msg("nceph: Configuration validation passed")

	// Create rados connection
	log.Debug().Str("client_id", conf.CephClientID).Msg("nceph: Creating rados connection")
	conn, err := rados2.NewConnWithUser(conf.CephClientID)
	if err != nil {
		log.Error().Err(err).Str("client_id", conf.CephClientID).Msg("nceph: Failed to create rados connection")
		return nil, errors.Wrap(err, "nceph: failed to create rados connection")
	}
	log.Debug().Msg("nceph: Rados connection created successfully")

	// Set configuration
	log.Debug().Str("config_file", conf.CephConfig).Msg("nceph: Reading ceph config file")
	err = conn.ReadConfigFile(conf.CephConfig)
	if err != nil {
		log.Error().Err(err).Str("config_file", conf.CephConfig).Msg("nceph: Failed to read ceph config file")
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to read ceph config file")
	}
	log.Debug().Str("config_file", conf.CephConfig).Msg("nceph: Config file read successfully")

	// Set keyring
	log.Debug().Str("keyring", conf.CephKeyring).Msg("nceph: Setting keyring for rados connection")
	err = conn.SetConfigOption("keyring", conf.CephKeyring)
	if err != nil {
		log.Error().Err(err).Str("keyring", conf.CephKeyring).Msg("nceph: Failed to set keyring")
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to set keyring")
	}
	log.Debug().Str("keyring", conf.CephKeyring).Msg("nceph: Keyring set successfully")

	// Connect to cluster
	log.Debug().Msg("nceph: Connecting to ceph cluster")
	err = conn.Connect()
	if err != nil {
		log.Error().Err(err).Msg("nceph: Failed to connect to ceph cluster")
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to connect to ceph cluster")
	}
	log.Debug().Msg("nceph: Successfully connected to ceph cluster")

	// Create cephfs mount
	log.Debug().Str("client_id", conf.CephClientID).Msg("nceph: Creating cephfs mount")
	fsMount, err := goceph.CreateMountWithId(conf.CephClientID)
	if err != nil {
		log.Error().Err(err).Str("client_id", conf.CephClientID).Msg("nceph: Failed to create cephfs mount")
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to create cephfs mount")
	}
	log.Debug().Msg("nceph: CephFS mount created successfully")

	// Configure mount with the same configuration
	log.Debug().Str("config_file", conf.CephConfig).Msg("nceph: Reading config file for cephfs mount")
	err = fsMount.ReadConfigFile(conf.CephConfig)
	if err != nil {
		log.Error().Err(err).Str("config_file", conf.CephConfig).Msg("nceph: Failed to read config for cephfs mount")
		fsMount.Release()
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to read config for mount")
	}
	log.Debug().Str("config_file", conf.CephConfig).Msg("nceph: Config file read successfully for cephfs mount")

	// Set keyring for mount
	log.Debug().Str("keyring", conf.CephKeyring).Msg("nceph: Setting keyring for cephfs mount")
	err = fsMount.SetConfigOption("keyring", conf.CephKeyring)
	if err != nil {
		log.Error().Err(err).Str("keyring", conf.CephKeyring).Msg("nceph: Failed to set keyring for cephfs mount")
		fsMount.Release()
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to set keyring for mount")
	}
	log.Debug().Str("keyring", conf.CephKeyring).Msg("nceph: Keyring set successfully for cephfs mount")

	// Mount the filesystem
	log.Debug().Msg("nceph: Mounting cephfs filesystem")
	err = fsMount.Mount()
	if err != nil {
		log.Error().Err(err).Msg("nceph: Failed to mount cephfs")
		fsMount.Release()
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to mount cephfs")
	}
	log.Debug().Msg("nceph: CephFS filesystem mounted successfully")

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
	log.Debug().Str("resourceId", id.OpaqueId).Msg("GetPathByID with CephFS implementation approach")

	// Convert resource ID to inode number
	inode, err := strconv.ParseInt(id.OpaqueId, 10, 64)
	if err != nil {
		return "", errors.Wrap(err, "nceph: invalid resource ID format")
	}

	// Get filesystem status to find active MDS
	activeMDS, err := fs.getActiveMDS(ctx)
	if err != nil {
		return "", errors.Wrap(err, "nceph: failed to get active MDS")
	}
	
	log.Debug().Str("active_mds", activeMDS).Int64("inode", inode).Msg("Found active MDS, dumping inode")

	// Dump inode information using the active MDS
	path, err := fs.dumpInode(ctx, activeMDS, inode)
	if err != nil {
		return "", errors.Wrap(err, "nceph: failed to dump inode")
	}

	// Remove ceph root prefix if configured
	if fs.conf.CephRoot != "" {
		path = strings.TrimPrefix(path, fs.conf.CephRoot)
	}

	// Ensure path starts with /
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	log.Debug().Str("path", path).Int64("inode", inode).Msg("Successfully resolved path by ID using CephFS approach")
	return path, nil
}

// dumpInode dumps inode information using Manager command (adapted for go-ceph library limitations)
func (fs *ncephfs) dumpInode(ctx context.Context, mdsSpec string, inode int64) (string, error) {
	log := appctx.GetLogger(ctx)
	
	// Since go-ceph doesn't have MdsCommand, we need to use MgrCommand with tell syntax
	// This sends a command to the MDS via the manager
	cmd, err := json.Marshal(map[string]interface{}{
		"prefix": "tell",
		"target": "mds." + mdsSpec,
		"args":   []string{"dump", "inode", strconv.FormatInt(inode, 10)},
		"format": "json",
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal tell mds dump inode command")
	}

	log.Debug().Str("command", string(cmd)).Str("mds", mdsSpec).Int64("inode", inode).Msg("Executing tell MDS command to dump inode")

	// Execute Manager command to tell MDS to dump inode
	buf, info, err := fs.cephAdminConn.radosConn.MgrCommand([][]byte{cmd})
	if err != nil {
		log.Warn().Err(err).Str("mds", mdsSpec).Int64("inode", inode).Msg("Tell MDS command failed")
		return "", errors.Wrap(err, "failed to execute tell MDS dump inode command")
	}

	if info != "" {
		log.Debug().Str("info", info).Msg("Tell MDS command info")
	}

	log.Debug().Str("mds_response", string(buf)).Msg("Raw tell MDS dump inode response")

	// Extract path from the MDS output
	path, err := fs.extractPathFromInodeOutput(ctx, buf)
	if err != nil {
		return "", errors.Wrap(err, "failed to extract path from inode output")
	}

	return path, nil
}

// extractPathFromInodeOutput extracts path from MDS dump inode output (based on CephFS implementation)
func (fs *ncephfs) extractPathFromInodeOutput(ctx context.Context, output []byte) (string, error) {
	log := appctx.GetLogger(ctx)
	
	// Try to parse as JSON first
	var inodeInfo map[string]interface{}
	if err := json.Unmarshal(output, &inodeInfo); err == nil {
		// Look for path information in the JSON response
		if path, ok := inodeInfo["path"].(string); ok && path != "" {
			log.Debug().Str("path", path).Msg("Extracted path from JSON inode info")
			return path, nil
		}
		
		// Look for other possible path fields
		pathFields := []string{"full_path", "pathname", "name", "dname"}
		for _, field := range pathFields {
			if path, ok := inodeInfo[field].(string); ok && path != "" {
				log.Debug().Str("field", field).Str("path", path).Msg("Extracted path from alternative JSON field")
				return path, nil
			}
		}
		
		log.Debug().Interface("inode_info", inodeInfo).Msg("JSON parsed but no path found in known fields")
	}

	// If JSON parsing fails, try text parsing (fallback approach)
	outputStr := string(output)
	log.Debug().Str("output", outputStr).Msg("Attempting text-based path extraction")
	
	// Look for common patterns in MDS output
	patterns := []string{
		`path\s*[:\s]+([^\s\n]+)`,
		`full_path\s*[:\s]+([^\s\n]+)`, 
		`pathname\s*[:\s]+([^\s\n]+)`,
		`"path"\s*:\s*"([^"]+)"`,
		`'path'\s*:\s*'([^']+)'`,
	}
	
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		
		matches := re.FindStringSubmatch(outputStr)
		if len(matches) > 1 && matches[1] != "" {
			path := strings.TrimSpace(matches[1])
			log.Debug().Str("pattern", pattern).Str("path", path).Msg("Extracted path using regex pattern")
			return path, nil
		}
	}

	return "", errors.New("no path information found in MDS output")
}

// getActiveMDS gets the active MDS using manager commands (based on CephFS implementation) 
func (fs *ncephfs) getActiveMDS(ctx context.Context) (string, error) {
	log := appctx.GetLogger(ctx)
	
	// Get filesystem status using the same approach as CephFS
	fsStatus, err := fs.getFSStatus(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get filesystem status")
	}
	
	// Parse active MDS from the status
	activeMDS, err := fs.parseActiveMDS(ctx, fsStatus)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse active MDS")
	}
	
	log.Debug().Str("active_mds", activeMDS).Msg("Found active MDS using CephFS approach")
	return activeMDS, nil
}

// getFSStatus gets filesystem status (based on CephFS implementation)
func (fs *ncephfs) getFSStatus(ctx context.Context) ([]byte, error) {
	// Prepare fs status command
	cmd, err := json.Marshal(map[string]interface{}{
		"prefix": "fs status",
		"format": "json",
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal fs status command")
	}

	// Execute manager command
	buf, info, err := fs.cephAdminConn.radosConn.MgrCommand([][]byte{cmd})
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute fs status command")
	}

	if info != "" {
		log := appctx.GetLogger(ctx)
		log.Debug().Str("info", info).Msg("Manager command info")
	}

	return buf, nil
}

// parseActiveMDS parses the active MDS from fs status output (based on CephFS implementation)
func (fs *ncephfs) parseActiveMDS(ctx context.Context, fsStatusOutput []byte) (string, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("fs_status_response", string(fsStatusOutput)).Msg("Parsing fs status for active MDS")
	
	// Parse the filesystem status JSON
	var fsStatus map[string]interface{}
	if err := json.Unmarshal(fsStatusOutput, &fsStatus); err != nil {
		return "", errors.Wrap(err, "failed to parse fs status JSON")
	}
	
	// Look for mdsmap
	mdsmapRaw, ok := fsStatus["mdsmap"]
	if !ok {
		return "", errors.New("no mdsmap found in fs status")
	}
	
	// Convert mdsmap to map
	mdsmapBytes, err := json.Marshal(mdsmapRaw)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal mdsmap")
	}
	
	var mdsmap map[string]interface{}
	if err := json.Unmarshal(mdsmapBytes, &mdsmap); err != nil {
		return "", errors.Wrap(err, "failed to unmarshal mdsmap")
	}
	
	// Look for info section
	infoRaw, ok := mdsmap["info"]
	if !ok {
		return "", errors.New("no info section found in mdsmap")
	}
	
	// Convert info to bytes for parsing
	infoBytes, err := json.Marshal(infoRaw)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal mdsmap info")
	}
	
	// Try parsing as map first (key-value pairs with MDS names as keys)
	var infoMap map[string]struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	
	if err := json.Unmarshal(infoBytes, &infoMap); err == nil {
		log.Debug().Int("mds_count", len(infoMap)).Msg("Parsed mdsmap.info as map")
		for _, mds := range infoMap {
			if strings.Contains(mds.State, "active") {
				log.Debug().Str("active_mds", mds.Name).Msg("Found active MDS in map format")
				return mds.Name, nil
			}
		}
	}
	
	// Try parsing as array (newer Ceph versions might use arrays)
	var infoArray []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	
	if err := json.Unmarshal(infoBytes, &infoArray); err == nil {
		log.Debug().Int("mds_count", len(infoArray)).Msg("Parsed mdsmap.info as array")
		for _, mds := range infoArray {
			if strings.Contains(mds.State, "active") {
				log.Debug().Str("active_mds", mds.Name).Msg("Found active MDS in array format")
				return mds.Name, nil
			}
		}
	}
	
	return "", errors.New("no active MDS found in mdsmap info")
}


