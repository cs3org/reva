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

package auth

import "io/fs"

// privateMode always holds. Windows does not keep Unix permission bits: a
// file reports 0666, or 0444 when read-only, so checking them would reject
// every cache and force a login on every command. What keeps the cache private
// there is the ACL on the user's profile, which only the user and
// administrators can read.
func privateMode(fs.FileMode) bool { return true }
