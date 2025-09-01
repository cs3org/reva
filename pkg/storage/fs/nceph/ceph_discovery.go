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
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/cs3org/reva/v3/pkg/appctx"
)

// CephMountInfo represents information discovered from system configuration
type CephMountInfo struct {
	MonitorHost     string // e.g., "cephminiflax.cern.ch:6789"
	CephVolumePath  string // e.g., "/volumes/_nogroup/rasmus"
	LocalMountPoint string // e.g., "/mnt/miniflax"
	ClientName      string // e.g., "mds-admin"
}

// DiscoverCephMountInfo attempts to auto-discover Ceph mount configuration from system files
func DiscoverCephMountInfo(ctx context.Context, cephConfigFile string) (*CephMountInfo, error) {
	log := appctx.GetLogger(ctx)

	log.Info().
		Str("ceph_config", cephConfigFile).
		Msg("nceph: Auto-discovering Ceph mount configuration from system files")

	// Step 1: Extract monitor host from Ceph config file
	monitorHost, err := extractMonitorHostFromConfig(ctx, cephConfigFile)
	if err != nil {
		log.Error().Err(err).Str("config_file", cephConfigFile).
			Msg("nceph: Failed to extract monitor host from Ceph config")
		return nil, fmt.Errorf("failed to extract monitor host from %s: %w", cephConfigFile, err)
	}

	log.Info().Str("monitor_host", monitorHost).
		Msg("nceph: Successfully extracted monitor host from Ceph config")

	// Step 2: Find matching fstab entry using the monitor host
	mountInfo, err := findCephMountInFstab(ctx, monitorHost)
	if err != nil {
		log.Error().Err(err).Str("monitor_host", monitorHost).
			Msg("nceph: Failed to find matching Ceph mount in fstab")
		return nil, fmt.Errorf("failed to find Ceph mount in fstab for monitor %s: %w", monitorHost, err)
	}

	log.Info().
		Str("monitor_host", mountInfo.MonitorHost).
		Str("ceph_volume_path", mountInfo.CephVolumePath).
		Str("local_mount_point", mountInfo.LocalMountPoint).
		Str("client_name", mountInfo.ClientName).
		Msg("nceph: Successfully auto-discovered Ceph mount configuration")

	return mountInfo, nil
}

// extractMonitorHostFromConfig extracts the monitor host from a Ceph config file
func extractMonitorHostFromConfig(ctx context.Context, configFile string) (string, error) {
	log := appctx.GetLogger(ctx)

	file, err := os.Open(configFile)
	if err != nil {
		return "", fmt.Errorf("failed to open Ceph config file %s: %w", configFile, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inGlobalSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inGlobalSection = (line == "[global]")
			continue
		}

		// Look for mon host in global section
		if inGlobalSection && strings.HasPrefix(line, "mon host") {
			// Parse "mon host = cephminiflax.cern.ch:6789"
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				monHost := strings.TrimSpace(parts[1])
				log.Debug().
					Str("config_line", line).
					Str("extracted_host", monHost).
					Msg("nceph: Extracted monitor host from config file")
				return monHost, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading Ceph config file: %w", err)
	}

	return "", fmt.Errorf("monitor host not found in Ceph config file %s", configFile)
}

// findCephMountInFstab searches fstab for a Ceph mount matching the given monitor host
func findCephMountInFstab(ctx context.Context, monitorHost string) (*CephMountInfo, error) {
	log := appctx.GetLogger(ctx)

	const fstabPath = "/etc/fstab"
	file, err := os.Open(fstabPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", fstabPath, err)
	}
	defer file.Close()

	// Regex to parse Ceph fstab entries:
	// Format: monitor:port:/volume/path /mount/point ceph options...
	// Example: cephminiflax.cern.ch:6789:/volumes/_nogroup/rasmus /mnt/miniflax ceph name=mds-admin,secretfile=...
	cephFstabRegex := regexp.MustCompile(`^([^:]+:\d+):([^\s]+)\s+([^\s]+)\s+ceph\s+(.+)`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matches := cephFstabRegex.FindStringSubmatch(line)
		if len(matches) == 5 {
			extractedMonitorHost := matches[1] // e.g., "cephminiflax.cern.ch:6789"
			cephVolumePath := matches[2]       // e.g., "/volumes/_nogroup/rasmus"
			localMountPoint := matches[3]      // e.g., "/mnt/miniflax"
			options := matches[4]              // e.g., "name=mds-admin,secretfile=..."

			log.Debug().
				Str("fstab_line", line).
				Str("monitor_host", extractedMonitorHost).
				Str("ceph_volume_path", cephVolumePath).
				Str("local_mount_point", localMountPoint).
				Msg("nceph: Parsed Ceph fstab entry")

			// Check if this matches our target monitor host
			if extractedMonitorHost == monitorHost {
				// Extract client name from options (e.g., "name=mds-admin")
				clientName := extractClientNameFromOptions(options)

				mountInfo := &CephMountInfo{
					MonitorHost:     extractedMonitorHost,
					CephVolumePath:  cephVolumePath,
					LocalMountPoint: localMountPoint,
					ClientName:      clientName,
				}

				log.Info().
					Str("matched_line", line).
					Interface("mount_info", mountInfo).
					Msg("nceph: Found matching Ceph mount in fstab")

				return mountInfo, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading fstab: %w", err)
	}

	return nil, fmt.Errorf("no Ceph mount found in fstab for monitor host %s", monitorHost)
}

// extractClientNameFromOptions extracts the client name from Ceph mount options
func extractClientNameFromOptions(options string) string {
	// Look for "name=client-name" in the options string
	nameRegex := regexp.MustCompile(`name=([^,\s]+)`)
	matches := nameRegex.FindStringSubmatch(options)
	if len(matches) == 2 {
		return matches[1]
	}
	return "admin" // Default fallback
}
