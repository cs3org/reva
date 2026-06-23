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

package service

// Base is embedded by services for convenience; Clients() returns the
// process-wide resolver installed by the runtime (see global.go).
type Base struct{}

func (Base) Clients() Clients { return Global() }

// MetadataProvider is implemented by a service that wants to advertise extra
// metadata (e.g. mount_id, public_url) on its registry node. The runtime merges
// RegistryMetadata() over the framework-derived keys at registration.
type MetadataProvider interface {
	RegistryMetadata() map[string]string
}
