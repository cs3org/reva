package appprovider

import (
	"maps"
	"net/http"
	"path"
	"slices"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/launcher"
	"github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc/metadata"
)

const maxOCMOpenBody = 64 << 10

// handleOCMOpen opens a shared resource for a remote OCM user. The remote
// server exchanged the share secret at /ocm/token, and its user's browser posts
// the resulting token here. The page answers with the form that hands the
// browser over to the app.
func (s *svc) handleOCMOpen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	nonce, err := launcher.NewNonce()
	if err != nil {
		launcher.WriteError(w, "", http.StatusInternalServerError, "Failed to open webapp.")
		return
	}
	fail := func(code int, msg string) {
		launcher.WriteError(w, nonce, code, msg)
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxOCMOpenBody)
	if err := r.ParseForm(); err != nil {
		fail(http.StatusBadRequest, "The launch request could not be read.")
		return
	}
	// Only the body is read, so the token cannot travel in a logged URL.
	accessToken := r.PostForm.Get("access_token")
	if accessToken == "" {
		fail(http.StatusUnauthorized, "The launch request is missing the access token.")
		return
	}

	rel := r.PathValue("rel")
	if rel != "" && (strings.Contains(rel, `\`) || slices.ContainsFunc(strings.Split(rel, "/"), func(seg string) bool {
		return seg == "" || seg == "." || seg == ".."
	})) {
		fail(http.StatusBadRequest, "The shared application cannot be launched.")
		return
	}

	gw, err := service.Gateway(ctx)
	if err != nil {
		log.Error().Err(err).Msg("ocm open: error getting gateway client")
		fail(http.StatusInternalServerError, "Failed to open webapp.")
		return
	}
	// The share is not named in the request: the token scope selects it.
	authRes, err := gw.Authenticate(ctx, &gateway.AuthenticateRequest{
		Type:         "ocmexchangedtoken",
		ClientSecret: accessToken,
	})
	switch {
	case err != nil:
		log.Error().Err(err).Msg("ocm open: authentication error")
		fail(http.StatusInternalServerError, "Failed to open webapp.")
		return
	case authRes.GetStatus().GetCode() != rpc.Code_CODE_OK:
		log.Info().Str("status", authRes.GetStatus().GetCode().String()).Msg("ocm open: token refused")
		fail(http.StatusUnauthorized, "The access token is not valid.")
		return
	}

	shareID := string(authRes.GetUser().GetOpaque().GetMap()["ocm-share-id"].GetValue())
	if shareID == "" {
		fail(http.StatusUnauthorized, "The access token is not valid.")
		return
	}
	ctx = appctx.ContextSetToken(ctx, authRes.Token)
	ctx = appctx.ContextSetUser(ctx, authRes.User)
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, authRes.Token)

	// The webapp permissions of the share decide, not the file permissions: a
	// view-only share cannot download the file but can still open it in the app.
	shareRes, err := gw.GetOCMShare(ctx, &ocm.GetOCMShareRequest{
		Ref: &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: shareID}}},
	})
	if err != nil || shareRes.GetStatus().GetCode() != rpc.Code_CODE_OK {
		log.Error().Err(err).Msg("ocm open: error getting the share")
		fail(http.StatusInternalServerError, "Failed to open webapp.")
		return
	}
	var perms *provider.ResourcePermissions
	for _, m := range shareRes.GetShare().GetAccessMethods() {
		if w := m.GetWebappOptions(); w != nil {
			perms = w.GetPermissions()
		}
	}
	var viewMode gateway.OpenInAppRequest_ViewMode
	switch {
	case perms.GetInitiateFileUpload():
		viewMode = gateway.OpenInAppRequest_VIEW_MODE_READ_WRITE
	case perms.GetInitiateFileDownload():
		viewMode = gateway.OpenInAppRequest_VIEW_MODE_READ_ONLY
	case perms.GetStat():
		viewMode = gateway.OpenInAppRequest_VIEW_MODE_VIEW_ONLY
	default:
		fail(http.StatusForbidden, "This application is not offered on the share.")
		return
	}

	// The gateway refuses an app that is not offered on the share.
	openRes, err := gw.OpenInApp(ctx, &gateway.OpenInAppRequest{
		Ref:      &provider.Reference{Path: path.Join("/ocm", shareID, rel)},
		ViewMode: viewMode,
		App:      r.PostForm.Get("appname"),
	})
	if err != nil {
		log.Error().Err(err).Msg("ocm open: error calling OpenInApp")
		fail(http.StatusInternalServerError, "Failed to open webapp.")
		return
	}
	switch openRes.GetStatus().GetCode() {
	case rpc.Code_CODE_OK:
	case rpc.Code_CODE_NOT_FOUND:
		fail(http.StatusNotFound, "The shared application was not found.")
		return
	case rpc.Code_CODE_PERMISSION_DENIED:
		fail(http.StatusForbidden, "This application is not offered on the share.")
		return
	default:
		log.Error().Str("status", openRes.GetStatus().GetCode().String()).Msg("ocm open: OpenInApp failed")
		fail(http.StatusInternalServerError, "Failed to open webapp.")
		return
	}

	appURL := openRes.GetAppUrl()
	if appURL.GetMethod() != http.MethodPost {
		launcher.SetHeaders(w, "", "")
		http.Redirect(w, r, appURL.GetAppUrl(), http.StatusSeeOther)
		return
	}
	action, origin, err := launcher.FormTarget(appURL.GetAppUrl())
	if err != nil {
		log.Error().Err(err).Msg("ocm open: app URL refused")
		fail(http.StatusBadRequest, "The shared application cannot be launched.")
		return
	}
	var fields []launcher.Field
	params := appURL.GetFormParameters()
	for _, name := range slices.Sorted(maps.Keys(params)) {
		fields = append(fields, launcher.Field{Name: name, Value: params[name]})
	}
	if err := launcher.WriteForm(w, nonce, action, origin, fields); err != nil {
		log.Error().Msg("ocm open: error writing launcher page")
	}
}
