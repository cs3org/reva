// Copyright 2018-2022 CERN
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

package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	group "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("mailer", New)
}

type config struct {
	SMTPAddress      string `mapstructure:"smtp_server" docs:";The hostname and port of the SMTP server."`
	SenderLogin      string `mapstructure:"sender_login" docs:";The email to be used to send mails."`
	SenderPassword   string `mapstructure:"sender_password" docs:";The sender's password."`
	DisableAuth      bool   `mapstructure:"disable_auth" docs:"false;Whether to disable SMTP auth."`
	Prefix           string `mapstructure:"prefix"`
	BodyTemplatePath string `mapstructure:"body_template_path"`
	SubjectTemplate  string `mapstructure:"subject_template"`
	GatewaySVC       string `mapstructure:"gateway_svc"`
}

type svc struct {
	conf    *config
	client  gateway.GatewayAPIClient
	tplBody *template.Template
	tplSubj *template.Template
}

// New creates a new mailer service.
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	conf.init()

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(conf.GatewaySVC))
	if err != nil {
		return nil, err
	}

	s := &svc{
		conf:   conf,
		client: client,
	}

	if err = s.initBodyTemplate(); err != nil {
		return nil, err
	}
	if err = s.initSubjectTemplate(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) initBodyTemplate() error {
	f, err := os.Open(s.conf.BodyTemplatePath)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	tpl, err := template.New("tpl_body").Parse(string(data))
	if err != nil {
		return err
	}

	s.tplBody = tpl
	return nil
}

func (s *svc) initSubjectTemplate() error {
	tpl, err := template.New("tpl_subj").Parse(s.conf.SubjectTemplate)
	if err != nil {
		return err
	}
	s.tplSubj = tpl
	return nil
}

func (c *config) init() {
	if c.Prefix == "" {
		c.Prefix = "mailer"
	}

	if c.SubjectTemplate == "" {
		c.SubjectTemplate = "{{.OwnerName}} ({{.OwnerUsername}}) shared {{if .IsDir}}folder{{else}}file{{end}} '{{.Filename}}' with you"
	}

	c.GatewaySVC = sharedconf.GetGatewaySVC(c.GatewaySVC)
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Unprotected() []string {
	return nil
}

type out struct {
	Recipients []string `json:"recipients"`
}

func getIDsFromRequest(r *http.Request) ([]string, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	idsSet := make(map[string]struct{})

	for _, id := range r.Form["id"] {
		if _, ok := idsSet[id]; ok {
			continue
		}
		idsSet[id] = struct{}{}
	}

	ids := make([]string, 0, len(idsSet))
	for id := range idsSet {
		ids = append(ids, id)
	}

	return ids, nil
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		ctx := r.Context()

		ids, err := getIDsFromRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

		if len(ids) == 0 {
			http.Error(w, "share id not provided", http.StatusBadRequest)
			return
		}

		var recipients []string
		for _, id := range ids {
			recipient, err := s.sendMailForShare(ctx, id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			recipients = append(recipients, recipient)
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Add("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out{Recipients: recipients})
	})
}

type shareInfo struct {
	RecipientEmail    string
	RecipientUsername string
	OwnerEmail        string
	OwnerName         string
	OwnerUsername     string
	ShareType         string
	Filename          string
	Path              string
	IsDir             bool
	ShareID           string
}

func (s *svc) getAuth() smtp.Auth {
	if s.conf.DisableAuth {
		return nil
	}
	return smtp.PlainAuth("", s.conf.SenderLogin, s.conf.SenderPassword, strings.SplitN(s.conf.SMTPAddress, ":", 2)[0])
}

func (s *svc) sendMailForShare(ctx context.Context, id string) (string, error) {
	share, err := s.getShareInfoByID(ctx, id)
	if err != nil {
		return "", err
	}

	msg, err := s.generateMsg(share.OwnerEmail, share.RecipientEmail, share)
	if err != nil {
		return "", err
	}

	return share.RecipientEmail, smtp.SendMail(s.conf.SMTPAddress, s.getAuth(), share.OwnerEmail, []string{share.RecipientEmail}, msg)
}

func (s *svc) generateMsg(from, to string, share *shareInfo) ([]byte, error) {
	subj, err := s.generateEmailSubject(share)
	if err != nil {
		return nil, err
	}

	body, err := s.generateEmailBody(share)
	if err != nil {
		return nil, err
	}

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n\r\n%s\r\n", from, to, subj, body)
	return []byte(msg), nil
}

func (s *svc) getShareInfoByID(ctx context.Context, id string) (*shareInfo, error) {
	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, errtypes.UserRequired("user not in context")
	}

	shareRes, err := s.client.GetShare(ctx, &collaboration.GetShareRequest{
		Ref: &collaboration.ShareReference{
			Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{
					OpaqueId: id,
				},
			},
		},
	})

	switch {
	case err != nil:
		return nil, err
	case shareRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return nil, errtypes.NotFound(fmt.Sprintf("share %s not found", id))
	case shareRes.Status.Code != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(shareRes.Status.Message)
	}

	share := shareRes.Share
	statRes, err := s.client.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: share.ResourceId,
		},
	})

	switch {
	case err != nil:
		return nil, err
	case statRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return nil, errtypes.NotFound("reference not found")
	case statRes.Status.Code != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(statRes.Status.Message)
	}

	file := statRes.Info

	info := &shareInfo{}
	switch g := share.Grantee.Id.(type) {
	case *provider.Grantee_UserId:
		grantee, err := s.getUser(ctx, g.UserId)
		if err != nil {
			return nil, err
		}
		info.RecipientEmail = grantee.Mail
		info.RecipientUsername = grantee.Username
		info.ShareType = "user"
	case *provider.Grantee_GroupId:
		grantee, err := s.getGroup(ctx, g.GroupId)
		if err != nil {
			return nil, err
		}
		info.RecipientEmail = grantee.Mail
		info.RecipientUsername = grantee.GroupName
		info.ShareType = "group"
	}

	info.OwnerEmail = user.Mail
	info.OwnerName = user.DisplayName
	info.OwnerUsername = user.Username

	info.Path = file.Path
	info.Filename = filepath.Base(file.Path)
	if file.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		info.IsDir = true
	} else {
		info.IsDir = false
	}

	info.ShareID = id

	return info, nil
}

func (s *svc) getUser(ctx context.Context, userID *user.UserId) (*user.User, error) {
	res, err := s.client.GetUser(ctx, &user.GetUserRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, err
	}

	return res.User, nil
}

func (s *svc) getGroup(ctx context.Context, groupID *group.GroupId) (*group.Group, error) {
	res, err := s.client.GetGroup(ctx, &group.GetGroupRequest{
		GroupId: groupID,
	})
	if err != nil {
		return nil, err
	}

	return res.Group, nil
}

func (s *svc) generateEmailSubject(share *shareInfo) (string, error) {
	var buf bytes.Buffer
	err := s.tplSubj.Execute(&buf, share)
	return buf.String(), err
}

func (s *svc) generateEmailBody(share *shareInfo) (string, error) {
	var buf bytes.Buffer
	err := s.tplBody.Execute(&buf, share)
	return buf.String(), err
}
