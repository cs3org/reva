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

// Package admin exposes the user-facing part of the Admin API over HTTP, for
// clients that only speak the public HTTPS surface. It covers what such a client
// needs and nothing more: whether the caller is an admin, and impersonation.
// Fleet operations stay on the gRPC Admin API.
package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func init() {
	global.Register("admin", New)
}

type config struct {
	Prefix string `mapstructure:"prefix"`
}

func (c *config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = "admin"
	}
}

type svc struct {
	conf   *config
	router *chi.Mux
}

// New returns the HTTP admin service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	s := &svc{conf: &c, router: chi.NewRouter()}
	s.router.Get("/status", s.handleStatus)
	s.router.Post("/impersonate", s.handleImpersonate)
	return s, nil
}

func (s *svc) Close() error { return nil }

func (s *svc) Prefix() string { return s.conf.Prefix }

func (s *svc) Unprotected() []string { return nil }

func (s *svc) Handler() http.Handler { return s.router }

type statusResponse struct {
	Admin bool `json:"admin"`
}

// handleStatus reports whether the caller may step up. It does not step up, so
// asking leaves no audit trail.
func (s *svc) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	client, err := service.Admin(ctx)
	if err != nil {
		writeUnavailable(ctx, w, err)
		return
	}
	res, err := client.CheckAdmin(ctx, &adminpb.CheckAdminRequest{})
	if err != nil {
		writeRPCError(ctx, w, err)
		return
	}
	writeJSON(ctx, w, http.StatusOK, statusResponse{Admin: res.Admin})
}

type impersonateRequest struct {
	User string `json:"user"`
	// Reason is optional, and recorded in the audit log when given.
	Reason string `json:"reason,omitempty"`
}

type impersonateResponse struct {
	Token string `json:"token"`
}

// handleImpersonate steps the caller up and impersonates the target in one
// request. The admin token is used for the Impersonate call and then dropped:
// it never leaves the server, so a client holds at most a user token, and the
// step-up is still checked and audited on every impersonation.
func (s *svc) handleImpersonate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req impersonateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeMessage(ctx, w, http.StatusBadRequest, "The request body is not valid JSON.")
		return
	}
	if req.User == "" {
		writeMessage(ctx, w, http.StatusBadRequest, "Name the user to impersonate.")
		return
	}

	client, err := service.Admin(ctx)
	if err != nil {
		writeUnavailable(ctx, w, err)
		return
	}
	elevated, err := client.RequestAdmin(ctx, &adminpb.RequestAdminRequest{})
	if err != nil {
		writeRPCError(ctx, w, err)
		return
	}
	res, err := client.Impersonate(withToken(ctx, elevated.Token), &adminpb.ImpersonateRequest{User: req.User, Reason: req.Reason})
	if err != nil {
		writeRPCError(ctx, w, err)
		return
	}
	writeJSON(ctx, w, http.StatusOK, impersonateResponse{Token: res.Token})
}

// withToken replaces the token carried to outgoing calls. The auth middleware
// has already put the caller's user token there, and a server reads only the
// first value, so appending would leave the user token in effect.
func withToken(ctx context.Context, tkn string) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(appctx.TokenHeader, tkn)
	ctx = appctx.ContextSetToken(ctx, tkn)
	return metadata.NewOutgoingContext(ctx, md)
}

// writeRPCError maps an Admin API status onto HTTP, keeping the distinction
// clients branch on: 401 when the caller is not authenticated, 403 when they
// are but are not an admin.
func writeRPCError(ctx context.Context, w http.ResponseWriter, err error) {
	st := status.Convert(err)
	switch st.Code() {
	case codes.Unauthenticated:
		writeMessage(ctx, w, http.StatusUnauthorized, "The server could not identify you.")
	case codes.PermissionDenied:
		writeMessage(ctx, w, http.StatusForbidden, "You are not an administrator of this server.")
	case codes.NotFound:
		writeMessage(ctx, w, http.StatusNotFound, "No such user.")
	case codes.InvalidArgument:
		writeMessage(ctx, w, http.StatusBadRequest, st.Message())
	case codes.FailedPrecondition:
		writeMessage(ctx, w, http.StatusNotImplemented, "Impersonation is not enabled on this server.")
	case codes.Unimplemented, codes.Unavailable:
		writeUnavailable(ctx, w, err)
	default:
		appctx.GetLogger(ctx).Error().Err(err).Msg("admin: rpc failed")
		writeMessage(ctx, w, http.StatusInternalServerError, "The admin request failed on the server.")
	}
}

func writeUnavailable(ctx context.Context, w http.ResponseWriter, err error) {
	appctx.GetLogger(ctx).Error().Err(err).Msg("admin: Admin API unavailable")
	writeMessage(ctx, w, http.StatusServiceUnavailable, "The Admin API is not available on this server.")
}

func writeMessage(ctx context.Context, w http.ResponseWriter, code int, msg string) {
	writeJSON(ctx, w, code, map[string]string{"message": msg})
}

func writeJSON(ctx context.Context, w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("admin: writing response")
	}
}
