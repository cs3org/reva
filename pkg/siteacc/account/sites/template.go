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

package sites

const tplJavaScript = `
function verifyForm(formData) {
	if (formData.getTrimmed("clientID") == "") {
		setState(STATE_ERROR, "Please enter the name of the test user.", "form", "clientID", true);
		return false;
	}

	if (formData.get("secret") == "") {
		setState(STATE_ERROR, "Please enter the password of the test user.", "form", "secret", true);
		return false;
	}

	return true;
}

function handleAction(action) {
	const formData = new FormData(document.querySelector("form"));
	if (!verifyForm(formData)) {
		return;
	}

	setState(STATE_STATUS, "Configuring sites... this should only take a moment.", "form", null, false);

	var xhr = new XMLHttpRequest();
    xhr.open("POST", "{{getServerAddress}}/" + action);
    xhr.setRequestHeader('Content-Type', 'application/json; charset=UTF-8');

	xhr.onload = function() {
		if (this.status == 200) {
			setState(STATE_SUCCESS, "Your sites was successfully configured!", "form", null, true);
		} else {
			var resp = JSON.parse(this.responseText);
			setState(STATE_ERROR, "An error occurred while trying to configure your sites:<br><em>" + resp.error + "</em>", "form", null, true);
		}
	}

	var postData = {
		"config": {
			"testClientCredentials": {
				"id": formData.getTrimmed("clientID"),
				"secret": formData.get("secret")
			}
		}
    };

    xhr.send(JSON.stringify(postData));
}
`

const tplStyleSheet = `
html * {
	font-family: arial !important;
}

input[type="checkbox"] {
	width: auto;
}

.mandatory {
	color: red;
	font-weight: bold;
}
`

const tplBody = `
<div>
	<p>Configure your ScienceMesh Sites below. <em>These settings affect the entire sites and not just your account.</em></p>
</div>
<div>&nbsp;</div>
<div>
	<form id="form" method="POST" class="box container-inline" style="width: 100%;" onSubmit="handleAction('sites-configure?invoker=user'); return false;">
		<div style="grid-row: 1; grid-column: 1 / span 2;">
			<h3>Test user settings</h3>
			<p>In order to perform automated tests on your sites, a test user has to be configured below for each site. Please note that the users <em>have to exist in your respective Reva instances</em>! If you do not have users for automated tests in your instances yet, create them first.</p>
			<hr>
		</div>

		{{range .Account.Operator.Sites}}
			<div style="grid-row: 2;"><label for="clientID">User name: <span class="mandatory">*</span></label></div>
			<div style="grid-row: 3;"><input type="text" id="clientID" name="clientID" placeholder="User name" value="{{.Config.TestClientCredentials.ID}}"/></div>
			<div style="grid-row: 2;"><label for="secret">Password: <span class="mandatory">*</span></label></div>
			<div style="grid-row: 3;"><input type="password" id="secret" name="secret" placeholder="Password" value="{{.Config.TestClientCredentials.Secret}}"/></div>
	
			<div style="grid-row: 4;">&nbsp;</div>
		{{end}}

		<div style="grid-row: 5; align-self: center;">
			Fields marked with <span class="mandatory">*</span> are mandatory.
		</div>
		<div style="grid-row: 5; grid-column: 2; text-align: right;">
			<button type="reset">Reset</button>
			<button type="submit" style="font-weight: bold;">Save</button>
		</div>
	</form>
</div>
<div>
	<p>Go <a href="{{getServerAddress}}/account/?path=manage">back</a> to the main account page.</p>
</div>
`
