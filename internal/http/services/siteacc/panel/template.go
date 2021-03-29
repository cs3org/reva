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

package panel

const panelTemplate = `
<!DOCTYPE html>
<html>
<head>	
	<script>
		function handleAction(action, email) {
			var xhr = new XMLHttpRequest();
	        xhr.open("POST", action);
	        xhr.setRequestHeader('Content-Type', 'application/json; charset=UTF-8');

			xhr.onreadystatechange = function() {
				if (this.readyState === XMLHttpRequest.DONE) {
					if (this.status == 200) {
						location.reload();	
					} else {
						console.log(this.responseText)
					}
	            }
			}
	        
			var postData = {
	            "email": email,
	        };

	        xhr.send(JSON.stringify(postData));
		}
	</script>
	<style>
		html * {
			font-family: monospace !important;
		}
		button {
			min-width: 140px;
		}
	</style>
	<title>Accounts panel</title>
</head>
<body>

<h1>Accounts ({{.Accounts | len}})</h1>
<p>
	<ul>
	{{range .Accounts}}
		<li>
			<p>
				<strong>{{.Email}}</strong><br>
				{{.FirstName}} {{.LastName}} <em>(Joined: {{.DateCreated.Format "Jan 02, 2006 15:04"}}; Last modified: {{.DateModified.Format "Jan 02, 2006 15:04"}})</em>
			</p>
			<p>
				<strong>API Key:</strong>
			{{if .Data.APIKey}}
				{{.Data.APIKey}}
			{{else}}
				<em>Not assigned</em>
			{{end}}
				<br>
				<strong>Site ID:</strong> {{.GetSiteID}}
				<br><br>
				<strong>Authorized:</strong>
			{{if .Data.Authorized}}
				<em>Yes</em>
			{{else}}
				<em>No</em>
			{{end}}	
			</p>
			<p>
				<form method="POST">
					<button type="button" onClick="handleAction('assign-api-key', '{{.Email}}');" {{if .Data.APIKey}}disabled{{end}}>Default API Key</button>
					<button type="button" onClick="handleAction('assign-api-key?isScienceMesh', '{{.Email}}');" {{if .Data.APIKey}}disabled{{end}}>ScienceMesh API Key</button>
	
				{{if .Data.Authorized}}
					<button type="button" onClick="handleAction('authorize?status=false', '{{.Email}}');" {{if not .Data.APIKey}}disabled{{end}}>Unauthorize</button>
				{{else}}
					<button type="button" onClick="handleAction('authorize?status=true', '{{.Email}}');" {{if not .Data.APIKey}}disabled{{end}}>Authorize</button>
				{{end}}

					<span style="width: 25px;">&nbsp;</span>
					<button type="button" onClick="handleAction('unregister-site', '{{.Email}}');" {{if not .Data.APIKey}}disabled{{end}}>Unregister site</button>

					<span style="width: 25px;">&nbsp;</span>
					<button type="button" onClick="handleAction('remove', '{{.Email}}');">Remove</button>
				</form>
			</p>
			<hr>
		</li>
	{{end}}
	</ul>
</p>

</body>
</html>
`
