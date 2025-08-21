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

package nceph

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	rados2 "github.com/ceph/go-ceph/rados"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/pkg/errors"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	goceph "github.com/ceph/go-ceph/cephfs"
)

// CephAdminConn represents an admin connection to Ceph with both rados and cephfs components
// adminMount is used for privileged operations like MDS commands
type CephAdminConn struct {
	radosConn  *rados2.Conn
	fsMount    *goceph.MountInfo
	adminMount *goceph.MountInfo // Admin mount for privileged MDS commands
}

// Close releases resources and closes the admin connection
func (conn *CephAdminConn) Close(ctx context.Context) error {
	var errs []error

	log := appctx.GetLogger(ctx)
	log.Debug().Msg("nceph: Closing admin ceph connection")

	if conn.adminMount != nil {
		log.Debug().Msg("nceph: Unmounting admin cephfs filesystem")
		if err := conn.adminMount.Unmount(); err != nil {
			log.Error().Err(err).Msg("nceph: Failed to unmount admin cephfs")
			errs = append(errs, errors.Wrap(err, "nceph: failed to unmount admin cephfs"))
		}
		
		log.Debug().Msg("nceph: Releasing admin mount")
		conn.adminMount.Release()
		conn.adminMount = nil
	}

	if conn.fsMount != nil {
		log.Debug().Msg("nceph: Unmounting cephfs filesystem")
		if err := conn.fsMount.Unmount(); err != nil {
			log.Error().Err(err).Msg("nceph: Failed to unmount cephfs")
			errs = append(errs, errors.Wrap(err, "nceph: failed to unmount cephfs"))
		}
		
		log.Debug().Msg("nceph: Releasing mount")
		conn.fsMount.Release()
		conn.fsMount = nil
	}

	if conn.radosConn != nil {
		log.Debug().Msg("nceph: Shutting down rados connection")
		conn.radosConn.Shutdown()
		conn.radosConn = nil
	}

	if len(errs) > 0 {
		log.Error().Int("error_count", len(errs)).Msg("nceph: Errors occurred while closing admin connection")
		return errs[0] // Return the first error
	}

	log.Debug().Msg("nceph: Admin ceph connection closed successfully")
	return nil
}

// mustMarshal is a helper function to marshal data to JSON, panicking on error
func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
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

	// Mount the filesystem with root
	log.Debug().Str("ceph_root", conf.CephRoot).Msg("nceph: Mounting cephfs filesystem with root")
	err = fsMount.MountWithRoot(conf.CephRoot)
	if err != nil {
		log.Error().Err(err).Str("ceph_root", conf.CephRoot).Msg("nceph: Failed to mount cephfs with root")
		fsMount.Release()
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to mount cephfs with root")
	}
	log.Debug().Msg("nceph: CephFS filesystem mounted successfully with root")

	// Create admin mount for MDS commands (following cephfs pattern)
	log.Debug().Str("client_id", conf.CephClientID).Msg("nceph: Creating admin mount for MDS commands")
	adminMount, err := goceph.CreateMountWithId(conf.CephClientID)
	if err != nil {
		log.Error().Err(err).Str("client_id", conf.CephClientID).Msg("nceph: Failed to create admin mount")
		fsMount.Unmount()
		fsMount.Release()
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to create admin mount")
	}
	log.Debug().Msg("nceph: Admin mount created successfully")

	// Configure admin mount with the same configuration
	log.Debug().Str("config_file", conf.CephConfig).Msg("nceph: Reading config file for admin mount")
	err = adminMount.ReadConfigFile(conf.CephConfig)
	if err != nil {
		log.Error().Err(err).Str("config_file", conf.CephConfig).Msg("nceph: Failed to read config for admin mount")
		adminMount.Release()
		fsMount.Unmount()
		fsMount.Release()
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to read config for admin mount")
	}

	// Set keyring for admin mount
	log.Debug().Str("keyring", conf.CephKeyring).Msg("nceph: Setting keyring for admin mount")
	err = adminMount.SetConfigOption("keyring", conf.CephKeyring)
	if err != nil {
		log.Error().Err(err).Str("keyring", conf.CephKeyring).Msg("nceph: Failed to set keyring for admin mount")
		adminMount.Release()
		fsMount.Unmount()
		fsMount.Release()
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to set keyring for admin mount")
	}

	// Mount the admin filesystem with root
	log.Debug().Str("ceph_root", conf.CephRoot).Msg("nceph: Mounting admin cephfs filesystem with root")
	err = adminMount.MountWithRoot(conf.CephRoot)
	if err != nil {
		log.Error().Err(err).Str("ceph_root", conf.CephRoot).Msg("nceph: Failed to mount admin cephfs with root")
		adminMount.Release()
		fsMount.Unmount()
		fsMount.Release()
		conn.Shutdown()
		return nil, errors.Wrap(err, "nceph: failed to mount admin cephfs with root")
	}
	log.Debug().Msg("nceph: Admin CephFS filesystem mounted successfully with root")

	log.Info().Msg("Successfully created ceph admin connection with admin mount for MDS commands")

	return &CephAdminConn{
		radosConn:  conn,
		fsMount:    fsMount,
		adminMount: adminMount,
	}, nil
}

func (fs *ncephfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	if fs.cephAdminConn == nil {
		return "", errtypes.NotSupported("nceph: GetPathByID requires ceph configuration")
	}

	log := appctx.GetLogger(ctx)
	log.Info().Str("resourceId", id.OpaqueId).Msg("nceph: Starting GetPathByID operation using MdsCommand dump inode")

	// Convert resource ID to inode number
	inode, err := strconv.ParseInt(id.OpaqueId, 10, 64)
	if err != nil {
		log.Error().Str("resourceId", id.OpaqueId).Err(err).Msg("nceph: Invalid resource ID format - must be numeric inode")
		return "", errors.Wrap(err, "nceph: invalid resource ID format")
	}

	log.Info().Int64("inode", inode).Msg("nceph: Successfully parsed resource ID to inode number")

	// Get filesystem status to find active MDS
	log.Info().Msg("nceph: Finding active MDS for inode operation")
	activeMDS, err := fs.getActiveMDS(ctx)
	if err != nil {
		log.Error().Err(err).Msg("nceph: Failed to find active MDS - cannot proceed with inode lookup")
		return "", errors.Wrap(err, "nceph: failed to get active MDS")
	}

	log.Info().Str("active_mds", activeMDS).Int64("inode", inode).Msg("nceph: Active MDS selected - proceeding with inode dump")

	// Dump inode information using the active MDS
	log.Info().Str("active_mds", activeMDS).Int64("inode", inode).Msg("nceph: Executing dump inode command")
	path, err := fs.dumpInode(ctx, activeMDS, inode)
	if err != nil {
		log.Error().Str("active_mds", activeMDS).Int64("inode", inode).Err(err).Msg("nceph: Dump inode command failed")
		return "", errors.Wrap(err, "nceph: failed to dump inode")
	}

	log.Info().Str("raw_path", path).Msg("nceph: Successfully extracted path from inode dump")

	// Remove ceph root prefix if configured
	originalPath := path
	if fs.conf.CephRoot != "" {
		path = strings.TrimPrefix(path, fs.conf.CephRoot)
		if originalPath != path {
			log.Info().Str("ceph_root", fs.conf.CephRoot).Str("original_path", originalPath).Str("trimmed_path", path).Msg("nceph: Removed CephRoot prefix from path")
		}
	}

	// Ensure path starts with /
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
		log.Info().Str("final_path", path).Msg("nceph: Added leading slash to path")
	}

	log.Info().Str("final_path", path).Int64("inode", inode).Str("active_mds", activeMDS).Msg("nceph: Successfully resolved path by ID using MdsCommand dump inode")
	return path, nil
}

// dumpInode uses the dump inode command to get inode information
func (fs *ncephfs) dumpInode(ctx context.Context, mdsName string, inode int64) (string, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("mds_name", mdsName).Int64("inode", inode).Msg("nceph: Preparing dump inode command")

	// Use dump inode command directly to the MDS via MdsCommand
	cmdData := map[string]interface{}{
		"prefix": "dump inode",
		"inode":  inode,
		"format": "json",
	}
	
	cmd, err := json.Marshal(cmdData)
	if err != nil {
		log.Error().Err(err).Interface("command_data", cmdData).Msg("nceph: Failed to marshal dump inode command")
		return "", errors.Wrap(err, "failed to marshal dump inode command")
	}

	log.Info().
		Str("command_json", string(cmd)).
		Str("mds_target", mdsName).
		Int64("target_inode", inode).
		Msg("nceph: Executing dump inode MdsCommand (direct to MDS)")

	// Use MdsCommand instead of MgrCommand for direct MDS communication
	buf, info, err := fs.cephAdminConn.adminMount.MdsCommand(mdsName, [][]byte{cmd})
	if err != nil {
		log.Error().
			Err(err).
			Str("mds_name", mdsName).
			Int64("inode", inode).
			Str("command", string(cmd)).
			Msg("nceph: MdsCommand failed - check MDS connectivity and inode validity")
		return "", errors.Wrap(err, "dump inode MdsCommand failed")
	}

	log.Info().
		Int("response_size", len(buf)).
		Bool("has_info", info != "").
		Str("mds_name", mdsName).
		Int64("inode", inode).
		Msg("nceph: Dump inode MdsCommand executed successfully")

	if info != "" {
		log.Info().Str("command_info", info).Msg("nceph: Additional info from dump inode MdsCommand")
	}

	log.Debug().
		Str("dump_inode_response", string(buf)).
		Str("mds_name", mdsName).
		Int64("inode", inode).
		Msg("nceph: Raw dump inode MdsCommand response")

	// Extract path from the dump inode output
	log.Info().Msg("nceph: Parsing dump inode response to extract path information")
	path, err := fs.extractPathFromInodeOutput(ctx, buf)
	if err != nil {
		log.Error().
			Err(err).
			Str("response", string(buf)).
			Int64("inode", inode).
			Msg("nceph: Failed to extract path from dump inode response")
		return "", errors.Wrap(err, "failed to extract path from dump inode output")
	}

	log.Info().
		Str("extracted_path", path).
		Int64("inode", inode).
		Str("mds_name", mdsName).
		Msg("nceph: Successfully extracted path from dump inode MdsCommand response")

	return path, nil
}
// extractPathFromInodeOutput extracts path from MDS dump inode output
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
	log := appctx.GetLogger(ctx)
	log.Info().Msg("nceph: Starting active MDS detection process")

	// Prepare fs status command
	cmd, err := json.Marshal(map[string]interface{}{
		"prefix": "fs status",
		"format": "json",
	})
	if err != nil {
		log.Error().Err(err).Msg("nceph: Failed to marshal fs status command")
		return "", errors.Wrap(err, "failed to marshal fs status command")
	}

	log.Debug().Str("command", "fs status").Msg("nceph: Executing fs status command to get cluster state")

	// Execute manager command (fs status is a manager command, not MDS command)
	buf, info, err := fs.cephAdminConn.radosConn.MgrCommand([][]byte{cmd})
	if err != nil {
		log.Error().Err(err).Msg("nceph: Failed to execute fs status command - check MDS cluster connectivity")
		return "", errors.Wrap(err, "failed to execute fs status command")
	}

	if info != "" {
		log.Debug().Str("info", info).Msg("nceph: Manager command returned additional info")
	}

	log.Debug().Str("fs_status_response", string(buf)).Msg("nceph: Raw fs status response received")
	log.Info().Int("response_size", len(buf)).Msg("nceph: Received fs status response, parsing for MDS information")

	// Parse the filesystem status JSON
	var fsStatus map[string]interface{}
	if err := json.Unmarshal(buf, &fsStatus); err != nil {
		log.Error().Err(err).Str("response", string(buf)).Msg("nceph: Failed to parse fs status as JSON")
		return "", errors.Wrap(err, "failed to parse fs status JSON")
	}

	log.Info().Int("fields_count", len(fsStatus)).Msg("nceph: Successfully parsed fs status JSON")
	
	// Log all top-level fields for debugging
	topLevelFields := make([]string, 0, len(fsStatus))
	for key := range fsStatus {
		topLevelFields = append(topLevelFields, key)
	}
	log.Debug().Strs("available_fields", topLevelFields).Msg("nceph: Available fields in fs status")

	// Look for mdsmap
	mdsmapRaw, ok := fsStatus["mdsmap"]
	if !ok {
		log.Error().Strs("available_fields", topLevelFields).Msg("nceph: No mdsmap field found in fs status - cluster may not have MDS services")
		return "", errors.New("no mdsmap found in fs status")
	}

	log.Info().Msg("nceph: Found mdsmap in fs status, analyzing MDS configuration")

	// Convert to JSON and back to handle the interface{}
	mdsmapBytes, err := json.Marshal(mdsmapRaw)
	if err != nil {
		log.Error().Err(err).Msg("nceph: Failed to marshal mdsmap for analysis")
		return "", errors.Wrap(err, "failed to marshal mdsmap")
	}

	log.Debug().Str("mdsmap_json", string(mdsmapBytes)).Int("mdsmap_size", len(mdsmapBytes)).Msg("nceph: Serialized mdsmap for parsing")

	// Try parsing as object with info field first (most common format)
	log.Info().Msg("nceph: Attempting to parse mdsmap as object with 'info' field")
	var mdsmap struct {
		Info json.RawMessage `json:"info"`
	}

	if err := json.Unmarshal(mdsmapBytes, &mdsmap); err == nil && len(mdsmap.Info) > 0 {
		log.Info().Int("info_size", len(mdsmap.Info)).Msg("nceph: Found 'info' field in mdsmap, parsing MDS entries")
		
		// Parse the info section as array first (newer Ceph format)
		var infoArray []struct {
			Name  string `json:"name"`
			State string `json:"state"`
			Rank  int    `json:"rank,omitempty"`
		}
		if err := json.Unmarshal(mdsmap.Info, &infoArray); err == nil {
			log.Info().Int("mds_count", len(infoArray)).Msg("nceph: Successfully parsed mdsmap.info as array format")
			
			for i, mds := range infoArray {
				log.Info().
					Int("mds_index", i).
					Str("mds_name", mds.Name).
					Str("mds_state", mds.State).
					Int("mds_rank", mds.Rank).
					Bool("is_active", strings.Contains(mds.State, "active")).
					Msg("nceph: Evaluating MDS entry from array")
				
				if strings.Contains(mds.State, "active") {
					log.Info().
						Str("chosen_mds", mds.Name).
						Str("mds_state", mds.State).
						Int("mds_rank", mds.Rank).
						Msg("nceph: SELECTED ACTIVE MDS - This MDS will be used for inode operations")
					return mds.Name, nil
				}
			}
			log.Warn().Int("total_mds", len(infoArray)).Msg("nceph: No active MDS found in array format - all MDS may be inactive")
		} else {
			log.Info().Msg("nceph: Array parsing failed, trying map format for mdsmap.info")
			// Try parsing as map (older Ceph format)
			var infoMap map[string]struct {
				Name  string `json:"name"`
				State string `json:"state"`
				Rank  int    `json:"rank,omitempty"`
			}
			if err := json.Unmarshal(mdsmap.Info, &infoMap); err == nil {
				log.Info().Int("mds_count", len(infoMap)).Msg("nceph: Successfully parsed mdsmap.info as map format")
				
				for key, mds := range infoMap {
					log.Info().
						Str("mds_key", key).
						Str("mds_name", mds.Name).
						Str("mds_state", mds.State).
						Int("mds_rank", mds.Rank).
						Bool("is_active", strings.Contains(mds.State, "active")).
						Msg("nceph: Evaluating MDS entry from map")
					
					if strings.Contains(mds.State, "active") {
						log.Info().
							Str("chosen_mds", mds.Name).
							Str("mds_state", mds.State).
							Int("mds_rank", mds.Rank).
							Str("mds_key", key).
							Msg("nceph: SELECTED ACTIVE MDS - This MDS will be used for inode operations")
						return mds.Name, nil
					}
				}
				log.Warn().Int("total_mds", len(infoMap)).Msg("nceph: No active MDS found in map format - all MDS may be inactive")
			} else {
				log.Error().Err(err).Str("info_raw", string(mdsmap.Info)).Msg("nceph: Failed to parse mdsmap.info as either array or map")
			}
		}
	} else {
		log.Info().Msg("nceph: No 'info' field found or empty, trying direct array parsing of mdsmap")
	}

	// If mdsmap.info approach fails, try direct array parsing (alternative format)
	log.Info().Msg("nceph: Attempting direct array parsing of mdsmap (alternative format)")
	var directMDSArray []struct {
		Name  string `json:"name"`
		State string `json:"state"`
		Rank  int    `json:"rank,omitempty"`
	}
	if err := json.Unmarshal(mdsmapBytes, &directMDSArray); err == nil {
		log.Info().Int("mds_entries", len(directMDSArray)).Msg("nceph: Successfully parsed mdsmap as direct array")
		
		for i, mds := range directMDSArray {
			log.Info().
				Int("mds_index", i).
				Str("mds_name", mds.Name).
				Str("mds_state", mds.State).
				Int("mds_rank", mds.Rank).
				Bool("is_active", strings.Contains(mds.State, "active")).
				Msg("nceph: Evaluating MDS entry from direct array")
			
			if strings.Contains(mds.State, "active") {
				log.Info().
					Str("chosen_mds", mds.Name).
					Str("mds_state", mds.State).
					Int("mds_rank", mds.Rank).
					Msg("nceph: SELECTED ACTIVE MDS - This MDS will be used for inode operations")
				return mds.Name, nil
			}
		}
		log.Warn().Int("total_mds", len(directMDSArray)).Msg("nceph: No active MDS found in direct array - all MDS may be inactive")
	} else {
		log.Error().Err(err).Str("mdsmap_raw", string(mdsmapBytes)).Msg("nceph: Failed to parse mdsmap as direct array")
	}

	log.Error().Msg("nceph: FAILED TO FIND ACTIVE MDS - No active MDS found in any format. Check MDS cluster health.")
	return "", errors.New("no active MDS found")
}
