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

package registration

const tplJavaScript = `
function verifyForm(formData) {
	if (formData.get("email") == "") {
		setState(STATE_ERROR, "Please specify your email address.", "form", "email", true);
		return false;
	}

	if (formData.get("fname") == "") {
		setState(STATE_ERROR, "Please specify your first name.", "form", "fname", true);
		return false;
	}

	if (formData.get("lname") == "") {
		setState(STATE_ERROR, "Please specify your last name.", "form", "lname", true);	
		return false;
	}

	if (formData.get("organization") == "") {
		setState(STATE_ERROR, "Please specify your organization/company.", "form", "organization", true);
		return false;
	}

	if (formData.get("password") == "") {
		setState(STATE_ERROR, "Please set a password.", "form", "password", true);
		return false;
	}

	if (formData.get("password2") == "") {
		setState(STATE_ERROR, "Please confirm your password.", "form", "password2", true);
		return false;
	}

	if (formData.get("password") != formData.get("password2")) {
		setState(STATE_ERROR, "The entered passwords do not match.", "form", "password2", true);
		return false;
	}

	return true;
}

function handleAction(action) {
	const formData = new FormData(document.querySelector("form"));
	if (!verifyForm(formData)) {
		return;
	}

	setState(STATE_STATUS, "Sending registration... this should only take a moment.", "form", null, false);

	var xhr = new XMLHttpRequest();
    xhr.open("POST", action);
    xhr.setRequestHeader('Content-Type', 'application/json; charset=UTF-8');

	xhr.onreadystatechange = function() {
		if (this.readyState === XMLHttpRequest.DONE) {
			if (this.status == 200) {
				setState(STATE_SUCCESS, "Your registration was successful! Please check your inbox for a confirmation email.");
			} else {
				var resp = JSON.parse(this.responseText);
				setState(STATE_ERROR, "An error occurred while trying to register your account:<br><em>" + resp.error + "</em>", "form", null, true);
			}
        }
	}

	var postData = {
        "email": formData.get("email"),
		"firstName": formData.get("fname"),
		"lastName": formData.get("lname"),
		"organization": formData.get("organization"),
		"website": formData.get("website"),
		"phoneNumber": formData.get("phone"),
		"password": {
			"value": formData.get("password")
		}
    };

    xhr.send(JSON.stringify(postData));
}
`

const tplStyleSheet = `
html * {
	font-family: arial !important;
}

.mandatory {
	color: red;
	font-weight: bold;
}
`

const tplBody = `
<div>
	<p>Fill out the form below to register for a ScienceMesh account. A confirmation email will be sent to you shortly after registration.</p>
	<p>
		After inspection by a ScienceMesh administrator, you will also receive an API key which can then be used in the
		<a href="https://github.com/sciencemesh/oc-sciencemesh" target="_blank">ownCloud</a> or
		<a href="https://github.com/sciencemesh/nc-sciencemesh" target="_blank">Nextcloud</a> web application.
	</p>
</div>
<div>&nbsp;</div>
<div>
	<form id="form" method="POST" class="box container-inline" style="width: 100%;">
		<div style="grid-row: 1;"><label for="email">Email address: <span class="mandatory">*</span></label></div>
		<div style="grid-row: 2;"><input type="text" id="email" name="email" placeholder="me@example.com"/></div>
		<div style="grid-row: 3;"><label for="fname">First name: <span class="mandatory">*</span></label></div>
		<div style="grid-row: 4;"><input type="text" id="fname" name="fname"/></div>
		<div style="grid-row: 3;"><label for="lname">Last name: <span class="mandatory">*</span></label></div>
		<div style="grid-row: 4;"><input type="text" id="lname" name="lname"/></div>

		<div style="grid-row: 5;"><label for="organization">Organization/Company: <span class="mandatory">*</span></label></div>
		<div style="grid-row: 6;"><input type="text" id="organization" name="organization"/></div>
		<div style="grid-row: 5;"><label for="website">Website:</label></div>
		<div style="grid-row: 6;"><input type="text" id="website" name="website" placeholder="https://www.example.com"/></div>

		<div style="grid-row: 7;"><label for="phone">Phone number:</label></div>
		<div style="grid-row: 8;"><input type="text" id="phone" name="phone" placeholder="+49 030 123456"/></div>

		<div style="grid-row: 9;">&nbsp;</div>

		<div style="grid-row: 10;"><label for="password">Password: <span class="mandatory">*</span></label></div>
		<div style="grid-row: 11;"><input type="password" id="password" name="password"/></div>
		<div style="grid-row: 10;"><label for="password2">Confirm password: <span class="mandatory">*</span></label></div>
		<div style="grid-row: 11;"><input type="password" id="password2" name="password2"/></div>

		<div style="grid-row: 12; font-style: italic; font-size: 0.8em;">
			The password must fulfil the following criteria:
			<ul style="margin-top: 0em;">
				<li>Must be at least 8 characters long</li>
				<li>Must contain at least 1 lowercase letter</li>
				<li>Must contain at least 1 uppercase letter</li>
				<li>Must contain at least 1 digit</li>
			</ul>
		</div>

		<div style="grid-row: 13; align-self: center;">
			Fields marked with <span class="mandatory">*</span> are mandatory.
		</div>
		<div style="grid-row: 13; grid-column: 2; text-align: right;">
			<button type="reset">Reset</button>
			<button type="button" style="font-weight: bold;" onClick="handleAction('create');">Register</button>
		</div>
	</form>	
</div>
`
