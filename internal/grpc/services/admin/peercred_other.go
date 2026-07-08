// Copyright 2018-2026 CERN
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

//go:build !linux

package admin

import "google.golang.org/grpc/credentials"

// peerCredentials is unavailable off Linux; startSocket rejects a configured
// socket before ever using it.
func peerCredentials() credentials.TransportCredentials { return nil }

// localRootSupported reports that SO_PEERCRED local root is Linux-only.
func localRootSupported() bool { return false }
