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

package manage

const tplJavaScript = `
function handleEditAccount() {
	setState(STATE_STATUS, "Redirecting to the account editor...");
	window.location.replace("{{getServerAddress}}/account/?path=edit");
}

function handleRequestAccess() {
	setState(STATE_STATUS, "Redirecting to the contact form...");		
	window.location.replace("{{getServerAddress}}/account/?path=contact&subject=" + encodeURIComponent("Request GOCDB access"));
}

function handleRequestKey() {
	setState(STATE_STATUS, "Redirecting to the contact form...");
	window.location.replace("{{getServerAddress}}/account/?path=contact&subject=" + encodeURIComponent("Request API key"));
}

function handleLogout() {
	var xhr = new XMLHttpRequest();
    xhr.open("GET", "{{getServerAddress}}/logout");
    xhr.setRequestHeader('Content-Type', 'application/json; charset=UTF-8');

	setState(STATE_STATUS, "Logging out...");

	xhr.onload = function() {
		if (this.status == 200) {
			setState(STATE_SUCCESS, "Done! Redirecting...");
			window.location.replace("{{getServerAddress}}/account/?path=login");
		} else {
			setState(STATE_ERROR, "An error occurred while logging out: " + this.responseText);
		}
	}
    
    xhr.send();
}
`

const tplStyleSheet = `
html * {
	font-family: arial !important;
}

.apikey {
	font-family: monospace !important;
	background: antiquewhite;
	border: 1px solid black;
	padding: 2px;
}
`

const tplBody = `
<div>
	<p><strong>Hello {{.Account.FirstName}} {{.Account.LastName}},</strong></p>
	<p>On this page, you can manage your ScienceMesh user account. This includes editing your personal information, requesting an API key or access to the GOCDB and more.</p>
</div>
<div>&nbsp;</div>
<div>
	<strong>Personal information:</strong>
	<ul style="margin-top: 0em;">
		<li>Name: <em>{{.Account.Title}}. {{.Account.FirstName}} {{.Account.LastName}}</em></li>
		<li>Email: <em><a href="mailto:{{.Account.Email}}">{{.Account.Email}}</a></em></li>
		<li>Organization/company: <em>{{.Account.Organization}} {{if .Account.Website}}(<a href="{{.Account.Website}}">{{.Account.Website}}</a>){{end}}</em></li>
		<li>Role: <em>{{.Account.Role}}</em></li>
		{{if .Account.PhoneNumber}}
		<li>Phone: <em>{{.Account.PhoneNumber}}</em></li>
		{{end}}
	</ul>
</div>
<div>
	<strong>Account data:</strong>
	<ul style="margin-top: 0em;">
		<li {{if .Account.Data.APIKey}}style="margin-bottom: 0.2em;"{{end}}>API Key: <em>{{if .Account.Data.APIKey}}<span class="apikey">{{.Account.Data.APIKey}}</span>{{else}}(no key assigned yet){{end}}</em></li>	
		<li>GOCDB access: <em>{{if .Account.Data.GOCDBAccess}}Granted{{else}}Not granted{{end}}</em></li>
	</ul>
</div>
<div>
	<form id="form" method="POST" class="box" style="width: 100%;">
		<button type="button" onClick="handleEditAccount();">Edit account</button>
		<span style="width: 25px;">&nbsp;</span>
		<button type="button" onClick="handleRequestKey();" {{if .Account.Data.APIKey}}disabled{{end}}>Request API Key</button>
		<button type="button" onClick="handleRequestAccess();" {{if .Account.Data.GOCDBAccess}}disabled{{end}}>Request GOCDB access</button>
		
		<button type="button" onClick="handleLogout();" style="float: right;">Logout</button>
	</form>
</div>
`
