// Copyright 2018-2026 CERN
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

package router

// Doc describes what a route takes and returns. It is attached where the route
// is declared, so the description cannot drift away from the thing it
// describes, and it is deliberately not serialised with the route: the table a
// service advertises in the registry says where a request goes, and is re-sent
// on every heartbeat, so it stays as small as routing needs it to be.
//
// A route needs no Doc. Without one it still appears in the generated
// documentation, with the path parameters its pattern names.
type Doc struct {
	// Summary is one line, shown next to the endpoint.
	Summary string
	// Description is the longer prose, shown when the endpoint is expanded.
	Description string
	// Params are the parameters the route takes outside its path. Path
	// parameters are not listed: the pattern already names them.
	Params []Param
	// Body is a value of the type the route accepts as its request body. Only
	// its type is read.
	Body any
	// Responses are values of the types the route replies with, by status
	// code. Only their types are read. A status with no body maps to nil.
	Responses map[int]any
}

// Param is a parameter a route takes outside its path.
type Param struct {
	Name        string
	In          string // "query" or "header"
	Description string
	Required    bool
	// Example is shown in the documentation, and is what a "try it" form is
	// prefilled with.
	Example string
}

// Query returns an optional query parameter.
func Query(name, description string) Param {
	return Param{Name: name, In: "query", Description: description}
}

// RequiredQuery returns a query parameter the route cannot do without.
func RequiredQuery(name, description string) Param {
	return Param{Name: name, In: "query", Description: description, Required: true}
}

// Header returns an optional header parameter.
func Header(name, description string) Param {
	return Param{Name: name, In: "header", Description: description}
}

// WithExample returns the parameter with an example value attached.
func (p Param) WithExample(example string) Param {
	p.Example = example
	return p
}

// Describe attaches documentation to a route.
func Describe(d Doc) Option {
	return func(r *Route) { r.Doc = &d }
}
