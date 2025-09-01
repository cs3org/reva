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
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/pkg/errors"
)

// FstabMountInfo contains all information extracted from a Ceph fstab entry
type FstabMountInfo struct {
	MonitorHost     string // e.g., "cephminiflax.cern.ch:6789"
	CephVolumePath  string // e.g., "/volumes/_nogroup/rasmus"
	LocalMountPoint string // e.g., "/mnt/miniflax"
	ClientName      string // e.g., "mds-admin"
	SecretFile      string // e.g., "/etc/ceph/miniflax.mds-admin.secret"
	ConfigFile      string // e.g., "/etc/ceph/miniflax.conf"
	KeyringFile     string // e.g., "/etc/ceph/miniflax.client.mds-admin.keyring"
}

// ParseFstabEntry parses a Ceph fstab entry and extracts all configuration information
//
// Example fstab entry:
// cephminiflax.cern.ch:6789:/volumes/_nogroup/rasmus	/mnt/miniflax	ceph	name=mds-admin,secretfile=/etc/ceph/miniflax.mds-admin.secret,x-systemd.device-timeout=30,x-systemd.mount-timeout=30,noatime,_netdev,wsync	0	2
func ParseFstabEntry(ctx context.Context, fstabLine string) (*FstabMountInfo, error) {
	log := appctx.GetLogger(ctx)

	if fstabLine == "" {
		return nil, errors.New("fstab entry is empty")
	}

	// Split the fstab line into fields (space or tab separated)
	fields := strings.Fields(fstabLine)
	if len(fields) < 4 {
		return nil, errors.New("invalid fstab entry: expected at least 4 fields")
	}

	device := fields[0]       // e.g., "cephminiflax.cern.ch:6789:/volumes/_nogroup/rasmus"
	mountPoint := fields[1]   // e.g., "/mnt/miniflax"
	fsType := fields[2]       // e.g., "ceph"
	options := fields[3]      // e.g., "name=mds-admin,secretfile=/etc/ceph/miniflax.mds-admin.secret,..."

	// Verify this is a Ceph mount
	if fsType != "ceph" {
		return nil, errors.Errorf("not a Ceph fstab entry: filesystem type is '%s', expected 'ceph'", fsType)
	}

	// Parse the device field: "monitor:port:/ceph/volume/path"
	deviceParts := strings.SplitN(device, ":", 3)
	if len(deviceParts) != 3 {
		return nil, errors.Errorf("invalid Ceph device format: '%s', expected 'monitor:port:/path'", device)
	}

	monitorHost := deviceParts[0] + ":" + deviceParts[1] // "cephminiflax.cern.ch:6789"
	cephVolumePath := deviceParts[2]                     // "/volumes/_nogroup/rasmus"

	log.Debug().
		Str("monitor_host", monitorHost).
		Str("ceph_volume_path", cephVolumePath).
		Str("local_mount_point", mountPoint).
		Msg("nceph: Parsed basic fstab entry components")

	// Parse mount options to extract client name and secret file
	var clientName, secretFile string
	optionList := strings.Split(options, ",")
	for _, option := range optionList {
		if strings.HasPrefix(option, "name=") {
			clientName = strings.TrimPrefix(option, "name=")
		} else if strings.HasPrefix(option, "secretfile=") {
			secretFile = strings.TrimPrefix(option, "secretfile=")
		}
	}

	if clientName == "" {
		return nil, errors.New("no 'name=' option found in fstab entry")
	}

	if secretFile == "" {
		return nil, errors.New("no 'secretfile=' option found in fstab entry")
	}

	// Derive other file paths based on the pattern from your example
	// From: /etc/ceph/miniflax.mds-admin.secret
	// Derive keyring and config files
	var configFile, keyringFile string
	
	// Try to extract hostname from secret file path for pattern-based derivation
	secretBasename := filepath.Base(secretFile)
	parts := strings.Split(secretBasename, ".")
	
	if len(parts) >= 3 {
		// Pattern: ceph.client.mds-admin.key -> hostname might be derivable
		hostname := parts[0] // First part might be hostname
		configFile = fmt.Sprintf("/etc/ceph/%s.conf", hostname)
		keyringFile = fmt.Sprintf("/etc/ceph/%s.client.%s.keyring", hostname, clientName)
	} else {
		// Fallback to standard patterns if derivation fails
		configFile = "/etc/ceph/ceph.conf"
		// For keyring, convert secretfile to keyring if it's a .key file
		if strings.HasSuffix(secretFile, ".key") {
			keyringFile = strings.TrimSuffix(secretFile, ".key") + ".keyring"
		} else {
			// Standard keyring pattern
			keyringFile = fmt.Sprintf("/etc/ceph/ceph.client.%s.keyring", clientName)
		}
	}

	mountInfo := &FstabMountInfo{
		MonitorHost:     monitorHost,
		CephVolumePath:  cephVolumePath,
		LocalMountPoint: mountPoint,
		ClientName:      clientName,
		SecretFile:      secretFile,
		ConfigFile:      configFile,
		KeyringFile:     keyringFile,
	}

	log.Info().
		Str("monitor_host", mountInfo.MonitorHost).
		Str("ceph_volume_path", mountInfo.CephVolumePath).
		Str("local_mount_point", mountInfo.LocalMountPoint).
		Str("client_name", mountInfo.ClientName).
		Str("config_file", mountInfo.ConfigFile).
		Str("keyring_file", mountInfo.KeyringFile).
		Str("secret_file", mountInfo.SecretFile).
		Msg("nceph: Successfully parsed fstab entry")

	return mountInfo, nil
}
