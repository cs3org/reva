// Copyright 2018-2020 CERN
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

package email

const accountCreatedTemplate = `
Dear {{.FirstName}} {{.LastName}},

Your ScienceMesh account has been successfully created!

An administrator will soon create an API key for your account; you will receive a separate email containing the key.

Kind regards,
The ScienceMesh Team
`

const apiKeyAssignedTemplate = `
Dear {{.FirstName}} {{.LastName}},

An API key has been created for your account:
{{.Data.APIKey}}

Keep this key in a safe and secret place!

Kind regards,
The ScienceMesh Team
`

const accountAuthorizedTemplate = `
Dear {{.FirstName}} {{.LastName}},

Congratulations - your site registration has been authorized!

Kind regards,
The ScienceMesh Team
`
