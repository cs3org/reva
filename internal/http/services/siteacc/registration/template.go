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

const formTemplate = `
<!DOCTYPE html>
<html>
<head>	
	<script>
		function enableForm(enable) {
			var form = document.getElementById("form");
			var elements = form.elements;
			for (var i = 0, len = elements.length; i < len; ++i) {
				elements[i].disabled = !enable;
			}
		}

		function setElementVisibility(id, visible) {
			var elem = document.getElementById(id);
			if (visible) {			
				elem.classList.add("visible");
				elem.classList.remove("hidden");
			} else {
				elem.classList.remove("visible");
				elem.classList.add("hidden");
			}
		}

		function setError(err, focusElem = "") {
			setElementVisibility("error", true);
			var elem = document.getElementById("error");
			elem.innerHTML = err;

			enableForm(true);

			if (focusElem != "") {
				var elem = document.getElementById(focusElem);
				elem.focus();
			}
		}

		function verifyForm(formData) {
			if (formData.get("email") == "") {
				setError("Please specify your email address.", "email");
				return false;
			}

			if (formData.get("fname") == "") {
				setError("Please specify your first name.", "fname");
				return false;
			}

			if (formData.get("lname") == "") {
				setError("Please specify your last name.", "lname");
				return false;
			}

			return true;
		}

		function handleAction(action) {
			const formData = new FormData(document.querySelector("form"));
			if (!verifyForm(formData)) {
				return;
			}

			enableForm(false);
			setElementVisibility("progress", true);
			setElementVisibility("success", false);
			setElementVisibility("error", false);

			var xhr = new XMLHttpRequest();
	        xhr.open("POST", action);
	        xhr.setRequestHeader('Content-Type', 'application/json; charset=UTF-8');

			xhr.onreadystatechange = function() {
				if (this.readyState === XMLHttpRequest.DONE) {
					setElementVisibility("progress", false);

					if (this.status == 200) {
						setElementVisibility("success", true);
					} else {
						var resp = JSON.parse(this.responseText);
						setError("An error occurred while trying to register your account:<br><em>" + resp.error + "</em>");
					}
	            }
			}
	
			var postData = {
	            "email": formData.get("email"),
				"firstName": formData.get("fname"),
				"lastName": formData.get("lname")
	        };

	        xhr.send(JSON.stringify(postData));
		}
	</script>
	<style>
		html * {
			font-family: arial !important;
		}
		form {
			border-color: lightgray !important;
		}
		button {
			min-width: 140px;
		}
		input {
			width: 95%;
		}
		label {
			font-weight: bold;
		}

		.box {
			width: 100%;
			border: 1px solid black;
			border-radius: 10px;
			padding: 10px;
		}
		.container {
			width: 900px;
			display: grid;
			grid-gap: 10px;
			position: absolute;
			left: 50%;
			transform: translate(-50%, 0);
		}
		.container-inline {
			display: inline-grid;
			grid-gap: 10px;
		}
		.mandatory {
			color: red;
			font-weight: bold;
		}
		.progress {
			border-color: #F7B22A;
			background: #FFEABF;
		}
		.success {
			border-color: #3CAC3A;
			background: #D3EFD2;
		}
		.error {
			border-color: #F20000;
			background: #F4D0D0;
		}
		.visible {
			display: block;
		}
		.hidden {
			display: none;
		}
	</style>
	<title>ScienceMesh Account Registration</title>
</head>
<body>

<div class="container">
	<div><h1>Welcome to the ScienceMesh Account Registration!</h1></div>
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

			<div style="grid-row: 5; align-self: center;">
				Fields marked with <span class="mandatory">*</span> are mandatory.
			</div>
			<div style="grid-row: 5; grid-column: 2; text-align: right;">
				<button type="reset">Reset</button>
				<button type="button" style="font-weight: bold;" onClick="handleAction('create');">Register</button>
			</div>
		</form>	
	</div>
	<div id="progress" class="box progress hidden">
		Sending registration...	this should only take a moment.
	</div>
	<div id="success" class="box success hidden">
		Your registration was successful! Please check your inbox for a confirmation email.
	</div>
	<div id="error" class="box error hidden">
	</div>
</div>
</body>
</html>
`
