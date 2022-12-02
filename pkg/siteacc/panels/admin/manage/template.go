// Copyright 2018-2022 CERN
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

package manage

const tplJavaScript = `
function handleViewAccounts() {
	setState(STATE_STATUS, "Redirecting to the accounts overview...");
	window.location.replace("{{getServerAddress}}/admin/?path=accounts");
}

function handleViewSites() {
	setState(STATE_STATUS, "Redirecting to the sites overview...");
	window.location.replace("{{getServerAddress}}/admin/?path=sites");
}
`

const tplStyleSheet = `
html * {
	font-family: arial !important;
}
button {
	min-width: 150px;
}
`

const tplBody = `
<div>
	<p><strong>Welcome to the ScienceMesh Site Administrators Mangement!</strong></p>
	<p>Using this service, you can manage all site administrator accounts as well as their corresponding sites.</p>
</div>
<div>
	<form id="form" method="POST" class="box" style="width: 100%;">
		<div>
			<button type="button" onClick="handleViewAccounts();">View accounts</button>
			<button type="button" onClick="handleViewSites();">View sites</button>	
		</div>	
	</form>
</div>
`
