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
	<p>There are currently <strong>{{.Operators | len}} operators</strong> stored in the system:</p>
</div>
<div style="font-size: 14px;">
	<ol style="padding-left: 25px;">
	{{range .Operators}}
		<li>
			<div>
				<div>
					<strong>{{.ID}}</strong> ({{.Sites | len}} site(s))<br>
				</div>
				<div>
					<ul style="padding-left: 20px; padding-top: 5px;">
					{{$parent := .}}
					{{range .Sites}}
						<li>
							<strong>{{getSiteName .ID true}}</strong> ({{.ID}})<br>
							{{if not .Config.TestClientCredentials.ID}}
							<em>Test user <strong>not</strong> configured!</em>
							{{end}}
						</li>				
					{{end}}
					</ul>
				</div>
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
