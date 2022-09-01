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

package accounts

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
	font-family: arial !important;
}
li::marker {
	font-weight: bold;
}
`

const tplBody = ` 
<div>
	<p>There are currently <strong>{{.Accounts | len}} accounts</strong> stored in the system:</p>
</div>
<div style="font-size: 14px;">
	<ol style="padding-left: 25px;">
	{{range .Accounts}}
		<li>
			<div>
				<div>
					<strong>{{.Email}}</strong><br>
					{{.Title}}. {{.FirstName}} {{.LastName}} <em>(Joined: {{.DateCreated.Format "Jan 02, 2006 15:04"}}; Last modified: {{.DateModified.Format "Jan 02, 2006 15:04"}})</em>
				</div>
				<div>
					<ul style="padding-left: 1em;">
						<li>
							<span>ScienceMesh Operator: {{getOperatorName .Operator}}</span>
							<br>
							<span style="margin-left: 20px; font-size: 90%;"><em>{{getOperatorSites .Operator true}}</em></span>
						</li>
						<li>Role: {{.Role}}</li>
						<li>Phone: {{.PhoneNumber}}</li>
					</ul>
				</div>
			</div>

			<div>&nbsp;</div>

			<div>
				<strong>Account data:</strong>
				<ul style="padding-left: 1em; padding-top: 0em;">	
					<li>Sites access: <em>{{if .Data.SitesAccess}}Granted{{else}}Not granted{{end}}</em></li>
					<li>GOCDB access: <em>{{if .Data.GOCDBAccess}}Granted{{else}}Not granted{{end}}</em></li>	
				</ul>
			</div>

			<div>&nbsp;</div>

			<div>
				<form method="POST" style="width: 100%;">
				{{if .Data.SitesAccess}}
					<button type="button" onClick="handleAction('grant-sites-access?status=false', '{{.Email}}');">Revoke Sites access</button>
				{{else}}
					<button type="button" onClick="handleAction('grant-sites-access?status=true', '{{.Email}}');">Grant Sites access</button>
				{{end}}

				{{if .Data.GOCDBAccess}}
					<button type="button" onClick="handleAction('grant-gocdb-access?status=false', '{{.Email}}');">Revoke GOCDB access</button>
				{{else}}
					<button type="button" onClick="handleAction('grant-gocdb-access?status=true', '{{.Email}}');">Grant GOCDB access</button>
				{{end}}

					<span style="width: 25px;">&nbsp;</span>
					<button type="button" onClick="handleAction('remove', '{{.Email}}');" style="float: right;">Remove</button>
				</form>
			</div>
			<hr>
		</li>
	{{end}}
	</ol>
</div>
<div>
	<p>Go <a href="{{getServerAddress}}/admin/?path=manage">back</a> to the main page.</p>
</div>
`
