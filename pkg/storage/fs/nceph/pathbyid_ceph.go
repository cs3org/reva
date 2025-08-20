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

// dumpInodeViaCommand uses MDS commands to dump inode information
func (fs *ncephfs) dumpInode(ctx context.Context, mdsName string, inode int64) (string, error) {
	log := appctx.GetLogger(ctx)

	// Try different MDS commands that might work to get inode path information

	// Method 1: Try inodes ls command to list inodes
	cmd1, err := json.Marshal(map[string]interface{}{
		"prefix": "inodes ls",
		"format": "json",
	})
	if err == nil {
		log.Debug().Str("command", "inodes ls").Msg("nceph: Trying inodes ls command")
		buf, info, err := fs.cephAdminConn.radosConn.MgrCommand([][]byte{cmd1})
		if err == nil {
			log.Debug().Str("inodes_ls_response", string(buf)).Msg("nceph: inodes ls response")
			// TODO: Parse the response to find the inode and extract path
			if info != "" {
				log.Debug().Str("info", info).Msg("inodes ls command info")
			}
		} else {
			log.Debug().Err(err).Msg("nceph: inodes ls command failed")
		}
	}

	// Method 2: Try dump inode command directly to the MDS
	cmd2, err := json.Marshal(map[string]interface{}{
		"prefix": "dump inode",
		"inode":  inode,
		"format": "json",
	})
	if err == nil {
		log.Debug().Str("command", "dump inode").Int64("inode", inode).Msg("nceph: Trying dump inode command")
		buf, info, err := fs.cephAdminConn.radosConn.MgrCommand([][]byte{cmd2})
		if err == nil {
			log.Debug().Str("dump_inode_response", string(buf)).Msg("nceph: dump inode response")
			// TODO: Parse the response to extract path
			if info != "" {
				log.Debug().Str("info", info).Msg("dump inode command info")
			}
		} else {
			log.Debug().Err(err).Msg("nceph: dump inode command failed")
		}
	}

	// Method 3: Try using MDS tell command instead of MgrCommand
	// This sends commands directly to the MDS daemon
	cmd3, err := json.Marshal(map[string]interface{}{
		"prefix": "tell",
		"target": "mds." + mdsName,
		"args":   []string{"dump", "inode", strconv.FormatInt(inode, 10)},
		"format": "json",
	})
	if err == nil {
		log.Debug().Str("command", "tell mds dump inode").Str("mds", mdsName).Int64("inode", inode).Msg("nceph: Trying tell mds dump inode command")
		buf, info, err := fs.cephAdminConn.radosConn.MgrCommand([][]byte{cmd3})
		if err == nil {
			log.Debug().Str("tell_mds_response", string(buf)).Msg("nceph: tell mds response")
			// TODO: Parse the response to extract path
			if info != "" {
				log.Debug().Str("info", info).Msg("tell mds command info")
			}
		} else {
			log.Debug().Err(err).Msg("nceph: tell mds command failed")
		}
	}

	// Method 4: Get MDS metadata first (original approach but with better logging)
	cmd4, err := json.Marshal(map[string]interface{}{
		"prefix": "mds metadata",
		"who":    mdsName,
		"format": "json",
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal mds metadata command")
	}

	log.Debug().Str("command", "mds metadata").Str("mds", mdsName).Msg("nceph: Trying mds metadata command")
	buf, info, err := fs.cephAdminConn.radosConn.MgrCommand([][]byte{cmd4})
	if err != nil {
		log.Warn().Err(err).Str("mds", mdsName).Msg("nceph: mds metadata command failed - this may indicate insufficient MDS admin permissions")
		return "", errors.Wrap(err, "failed to execute mds metadata command")
	}

	if info != "" {
		log.Debug().Str("info", info).Msg("MDS metadata command info")
	}

	log.Debug().
		Str("mds", mdsName).
		Int64("inode", inode).
		Str("mds_metadata_response", string(buf)).
		Msg("nceph: MDS metadata response - inode resolution not yet fully implemented")

	// For now, indicate that we got the MDS metadata but full inode->path resolution
	// requires additional MDS API integration that's not yet implemented
	return "", errors.New("inode to path resolution requires MDS admin access - tried multiple MDS command approaches but none succeeded yet")
}// extractPathFromInodeOutput extracts path from MDS dump inode output (based on CephFS implementation)
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

	// Add debug logging to see the raw response
	log := appctx.GetLogger(ctx)
	log.Debug().Str("fs_status_response", string(buf)).Msg("nceph: Raw fs status response for debugging")

	// Try to determine the response format by looking at the first character
	trimmed := strings.TrimSpace(string(buf))
	if len(trimmed) == 0 {
		return "", errors.New("empty fs status response")
	}

	var activeMDS string
	var parseErr error

	// Check if response starts with '[' (array) or '{' (object)
	if strings.HasPrefix(trimmed, "[") {
		log.Debug().Msg("nceph: fs status response appears to be an array")
		// Handle array response format
		var responseArray []map[string]interface{}
		if err := json.Unmarshal(buf, &responseArray); err != nil {
			parseErr = errors.Wrap(err, "failed to parse fs status array response")
		} else {
			// Look for mdsmap in array elements
			for _, item := range responseArray {
				if mdsmap, ok := item["mdsmap"]; ok {
					activeMDS, parseErr = fs.extractActiveMDSFromMap(ctx, mdsmap)
					if parseErr == nil && activeMDS != "" {
						break
					}
				}
			}
			if activeMDS == "" && parseErr == nil {
				parseErr = errors.New("no mdsmap found in array response")
			}
		}
	} else if strings.HasPrefix(trimmed, "{") {
		log.Debug().Msg("nceph: fs status response appears to be an object")
		// Handle object response format (original approach)
		activeMDS, parseErr = fs.parseObjectFormatResponse(ctx, buf)
	} else {
		parseErr = errors.New("fs status response format not recognized - does not start with { or [")
	}

	if parseErr != nil {
		return "", parseErr
	}

	if activeMDS == "" {
		return "", errors.New("no active MDS found in response")
	}

	return activeMDS, nil
}

// parseObjectFormatResponse handles the original object-format response
func (fs *ncephfs) parseObjectFormatResponse(ctx context.Context, buf []byte) (string, error) {
	// Parse the filesystem status JSON
	var fsStatus map[string]interface{}
	if err := json.Unmarshal(buf, &fsStatus); err != nil {
		return "", errors.Wrap(err, "failed to parse fs status JSON")
	}

	// Look for mdsmap
	mdsmapRaw, ok := fsStatus["mdsmap"]
	if !ok {
		return "", errors.New("no mdsmap found in fs status")
	}

	// The key issue: mdsmap can be either an object with info, or directly an array
	return fs.extractActiveMDSFromMap(ctx, mdsmapRaw)
}

// extractActiveMDSFromMap extracts active MDS from a generic map (used for both array and object responses)
func (fs *ncephfs) extractActiveMDSFromMap(ctx context.Context, mdsmapInterface interface{}) (string, error) {
	log := appctx.GetLogger(ctx)

	// Convert to JSON and back to handle the interface{}
	mdsmapBytes, err := json.Marshal(mdsmapInterface)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal mdsmap from interface")
	}

	log.Debug().Str("mdsmap_json", string(mdsmapBytes)).Msg("nceph: mdsmap JSON for parsing")

	// Check if mdsmap is directly an array (your case)
	var directMDSArray []struct {
		Name  string `json:"name"`
		State string `json:"state"`
		Rank  int    `json:"rank,omitempty"`
	}
	
	if err := json.Unmarshal(mdsmapBytes, &directMDSArray); err == nil {
		log.Debug().Int("mds_entries", len(directMDSArray)).Msg("nceph: Successfully parsed mdsmap as direct array")
		for i, mds := range directMDSArray {
			log.Debug().Int("mdsmap_index", i).Str("mds_name", mds.Name).Str("mds_state", mds.State).Msg("nceph: Processing direct mdsmap array element")
			if strings.Contains(mds.State, "active") {
				log.Debug().Str("active_mds", mds.Name).Msg("nceph: Found active MDS in direct array")
				return mds.Name, nil
			}
		}
		return "", errors.New("no active MDS found in direct mdsmap array")
	}

	// If not a direct array, try parsing as object with info field
	var mdsmap struct {
		Info json.RawMessage `json:"info"`
	}

	if err := json.Unmarshal(mdsmapBytes, &mdsmap); err != nil {
		return "", errors.Wrap(err, "failed to unmarshal mdsmap as object with info")
	}

	// Parse the info section
	return fs.extractActiveMDSFromRawInfo(ctx, mdsmap.Info)
}

// extractActiveMDSFromRawInfo extracts active MDS from raw info JSON (handles both array and object)
func (fs *ncephfs) extractActiveMDSFromRawInfo(ctx context.Context, infoRaw json.RawMessage) (string, error) {
	log := appctx.GetLogger(ctx)

	// First, try to parse as array (newer Ceph versions)
	var infoArray []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(infoRaw, &infoArray); err == nil {
		log.Debug().Int("mds_count", len(infoArray)).Msg("nceph: Parsed mdsmap.info as array")
		for _, mds := range infoArray {
			if strings.Contains(mds.State, "active") {
				return mds.Name, nil
			}
		}
	} else {
		// Try to parse as map/object (older Ceph versions)
		var infoMap map[string]struct {
			Name  string `json:"name"`
			State string `json:"state"`
		}
		if err := json.Unmarshal(infoRaw, &infoMap); err == nil {
			log.Debug().Int("mds_count", len(infoMap)).Msg("nceph: Parsed mdsmap.info as map")
			for _, mds := range infoMap {
				if strings.Contains(mds.State, "active") {
					return mds.Name, nil
				}
			}
		} else {
			return "", errors.Wrap(err, "failed to parse mdsmap.info as either array or map")
		}
	}

	return "", errors.New("no active MDS found in mdsmap info")
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
