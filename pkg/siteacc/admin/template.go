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

package admin

const tplJavaScript = `
function handleAction(action, email) {
	var xhr = new XMLHttpRequest();
    xhr.open("POST", "{{getServerAddress}}/" + action);
    xhr.setRequestHeader('Content-Type', 'application/json; charset=UTF-8');

	setState(STATE_STATUS, "Performing request...");

	xhr.onload = function() {
		if (this.status == 200) {
			setState(STATE_SUCCESS, "Done! Reloading...");
			location.reload();
		} else {
			setState(STATE_ERROR, "An error occurred while performing the request: " + this.responseText);
		}
	}
    
	var postData = {
        "email": email,
    };

    xhr.send(JSON.stringify(postData));
}
`

const tplStyleSheet = `
html * {
	font-family: monospace !important;
}
`

const tplBody = `
<div>
	<ul>
	{{range .Accounts}}
		<li>
			<p>
				<div>
					<strong>{{.Email}}</strong><br>
					{{.Title}}. {{.FirstName}} {{.LastName}} <em>(Joined: {{.DateCreated.Format "Jan 02, 2006 15:04"}}; Last modified: {{.DateModified.Format "Jan 02, 2006 15:04"}})</em>
				</div>
				<div>
					<ul style="padding-left: 1em;">
						<li>ScienceMesh Site: {{getSiteName .Site false}} ({{getSiteName .Site true}})</li>
						<li>Role: {{.Role}}</li>
						<li>Phone: {{.PhoneNumber}}</li>
					</ul>
				</div>
			</p>
			<p>
			<!--
				<strong>API Key:</strong> {{if .Data.APIKey}}{{.Data.APIKey}}{{else}}<em>Not assigned</em>{{end}}
				<br>
				<strong>Site ID:</strong> {{.GetSiteID}}	
				<br>
			-->
				<strong>GOCDB access:</strong> <em>{{if .Data.GOCDBAccess}}Granted{{else}}Not granted{{end}}</em>
			</p>
			<p>
				<form method="POST" style="width: 100%;">
				<!--
					<button type="button" onClick="handleAction('assign-api-key', '{{.Email}}');" {{if .Data.APIKey}}disabled{{end}}>Default API Key</button>
					<button type="button" onClick="handleAction('assign-api-key?isScienceMesh', '{{.Email}}');" {{if .Data.APIKey}}disabled{{end}}>ScienceMesh API Key</button>
					<br><br>
				-->

				{{if .Data.GOCDBAccess}}
					<button type="button" onClick="handleAction('grant-gocdb-access?status=false', '{{.Email}}');">Revoke GOCDB access</button>
				{{else}}
					<button type="button" onClick="handleAction('grant-gocdb-access?status=true', '{{.Email}}');">Grant GOCDB access</button>
				{{end}}
	
				<!--
					<span style="width: 25px;">&nbsp;</span>
					<button type="button" onClick="handleAction('unregister-site', '{{.Email}}');" {{if not .Data.APIKey}}disabled{{end}}>Unregister site</button>
				-->

					<span style="width: 25px;">&nbsp;</span>
					<button type="button" onClick="handleAction('remove', '{{.Email}}');" style="float: right;">Remove</button>
				</form>
			</p>
			<hr>
		</li>
	{{end}}
	</ul>
</div>
`
