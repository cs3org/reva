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

package mcp

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolListSpaces       = "list_spaces"
	toolListFolder       = "list_folder"
	toolStat             = "stat"
	toolReadFile         = "read_file"
	toolWebLink          = "web_link"
	toolListShares       = "list_shares"
	toolCreateShare      = "create_share"
	toolCreatePublicLink = "create_public_link"
	toolRemoveShare      = "remove_share"
	toolListVersions     = "list_versions"
	toolRestoreVersion   = "restore_version"
	toolListTrash        = "list_trash"
	toolRestoreFromTrash = "restore_from_trash"
	toolGetLabels        = "get_labels"
	toolSetLabels        = "set_labels"
	toolCreateFolder     = "create_folder"
	toolWriteFile        = "write_file"
	toolMove             = "move"
	toolDelete           = "delete"
)

func (s *svc) registerTools() {
	s.tools = map[string]*toolInfo{}
	s.registerFileTools()
	s.registerShareTools()
	s.registerHistoryTools()
}

// toolInfo is what the service needs to know of a tool besides its handler.
type toolInfo struct {
	// permissions are all needed. Every tool also needs files:list, since the
	// gateway resolves every reference through the storage registry first.
	permissions []string
	write       bool
}

// call is the state of one tool call.
type call struct {
	ctx  context.Context
	gw   gateway.GatewayAPIClient
	ap   *scope.AppAuthScope
	user *userpb.User

	// For the audit log.
	path, destination string
	bytes             int64
	// content is returned along with the structured output.
	content []sdk.Content
}

// toolSpec declares a tool.
type toolSpec struct {
	name        string
	description string
	permissions []string
	readOnly    bool
	destructive bool
	// enums restricts string properties of the input.
	enums map[string][]any
}

// addTool registers a tool whose handler runs only when the app password
// grants its permissions, and logs every call.
func addTool[In, Out any](s *svc, spec toolSpec, h func(c *call, in In) (Out, error)) {
	if slices.Contains(s.c.DisabledTools, spec.name) {
		return
	}
	info := &toolInfo{permissions: append([]string{scope.PermFilesList}, spec.permissions...), write: !spec.readOnly}
	s.tools[spec.name] = info

	in, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Sprintf("mcp: input schema of %s: %v", spec.name, err))
	}
	for prop, values := range spec.enums {
		in.Properties[prop].Enum = values
	}
	tool := &sdk.Tool{
		Name:        spec.name,
		Description: spec.description,
		InputSchema: in,
		Annotations: &sdk.ToolAnnotations{
			ReadOnlyHint:    spec.readOnly,
			DestructiveHint: &spec.destructive,
			OpenWorldHint:   new(false),
		},
	}
	sdk.AddTool(s.server, tool, func(ctx context.Context, _ *sdk.CallToolRequest, input In) (*sdk.CallToolResult, Out, error) {
		start := time.Now()
		c := &call{ctx: ctx}
		var out Out
		err := c.prepare(info)
		if err == nil {
			out, err = h(c, input)
		}

		log := appctx.GetLogger(ctx).Info()
		if c.ap != nil {
			log = log.Str("password_id", c.ap.PasswordID)
		}
		log.Str("tool", spec.name).Str("path", c.path).Str("destination", c.destination).Str("result", errorCode(err)).
			Int64("bytes", c.bytes).Dur("duration", time.Since(start)).Msg("mcp tool call")

		if err != nil {
			var zero Out
			return nil, zero, asToolError(err, spec.name)
		}
		var res *sdk.CallToolResult
		if c.content != nil {
			res = &sdk.CallToolResult{Content: c.content}
		}
		return res, out, nil
	})
}

// prepare checks that the app password grants the tool, and gets what the
// handler needs from the request context.
func (c *call) prepare(info *toolInfo) error {
	ap, ok := scope.AppAuthFromContext(c.ctx)
	if !ok || ap.Client != scope.ClientMCP {
		return newError(codeNotPermittedByAP, "an mcp app password is required")
	}
	c.ap = ap
	if missing := missingPermissions(ap, info); len(missing) > 0 {
		return newError(codeNotPermittedByAP, "the app password does not grant "+strings.Join(missing, ", "))
	}
	user, ok := appctx.ContextGetUser(c.ctx)
	if !ok {
		return newError(codeInternal, "no user in the request")
	}
	c.user = user
	gw, err := service.Gateway(c.ctx)
	if err != nil {
		return newError(codeInternal, "gateway unavailable: "+err.Error())
	}
	c.gw = gw
	return nil
}

func missingPermissions(ap *scope.AppAuthScope, info *toolInfo) []string {
	var missing []string
	for _, p := range info.permissions {
		if !ap.Has(p) {
			missing = append(missing, p)
		}
	}
	return missing
}

// filterToolList offers only the tools the app password allows. It is an aid
// for the model; every handler checks again.
func (s *svc) filterToolList(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		res, err := next(ctx, method, req)
		list, ok := res.(*sdk.ListToolsResult)
		if err != nil || method != "tools/list" || !ok {
			return res, err
		}
		ap, ok := scope.AppAuthFromContext(ctx)
		filtered := *list
		filtered.Tools = slices.DeleteFunc(slices.Clone(list.Tools), func(t *sdk.Tool) bool {
			info, known := s.tools[t.Name]
			return !ok || ap.Client != scope.ClientMCP || !known || len(missingPermissions(ap, info)) > 0
		})
		return &filtered, nil
	}
}

// absPath checks a path given by the client: absolute, and under the roots of
// the app password.
func (c *call) absPath(p, arg string) (string, error) {
	if !path.IsAbs(p) {
		return "", newError(codeInvalidArgument, fmt.Sprintf("%s must be an absolute CERNBox path, such as /eos/user/j/jdoe/file.txt; got %q (list_spaces shows where to start)", arg, p))
	}
	p = path.Clean(p)
	if !c.ap.UnderRoots(p) {
		return "", newError(codeOutsideRoots, fmt.Sprintf("%s is outside the folders this app password is restricted to: %s", p, strings.Join(c.ap.Roots, ", ")))
	}
	return p, nil
}
