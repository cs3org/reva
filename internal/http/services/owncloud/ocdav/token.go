package ocdav

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/rhttp/router"
	"google.golang.org/grpc/metadata"
)

// TokenHandler handles requests for public link tokens
type TokenHandler struct{}

// Handler handles http requests
func (t TokenHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("TOKEN Handler")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		typ, tkn := router.ShiftPath(r.URL.Path)
		tkn, _ = router.ShiftPath(tkn)

		c, err := pool.GetGatewayServiceClient(s.c.GatewaySvc)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		switch typ {
		case "protected":
			s.handleGetToken(w, r, tkn, c, true)
		case "unprotected":
			s.handleGetToken(w, r, tkn, c, false)
		}
	})
}

// TokenInfo contains information about the token
type TokenInfo struct {
	Token             string `xml:"token"`
	LinkURL           string `xml:"linkurl"`
	PasswordProtected bool   `xml:"passwordprotected"`

	StorageID string `xml:"storageid"`
	OpaqueID  string `xml:"opaqueid"`
	Path      string `xml:"path"`
}

func (s *svc) handleGetToken(w http.ResponseWriter, r *http.Request, tkn string, c gateway.GatewayAPIClient, protected bool) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	user, token, passwordProtected, err := getInfoForToken(tkn, r.URL.Query(), c)
	if err != nil {
		log.Error().Err(err).Msg("error stating token")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	t, err := buildTokenInfo(user, tkn, token, passwordProtected, c)
	if err != nil {
		log.Error().Err(err).Msg("error stating resource behind token")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if protected {
		if t.PasswordProtected == true {
			log.Error().Msg("password protected private links are not supported")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ref := &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: t.StorageID,
				OpaqueId:  t.OpaqueID,
			},
			Path: "",
		}
		res, err := c.Stat(ctx, &provider.StatRequest{
			Ref: ref,
		})
		fmt.Println("FILE STAT", res, err)
	}

	b, err := xml.Marshal(t)
	if err != nil {
		log.Error().Err(err).Msg("error marshaling xml")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(b)
	w.WriteHeader(http.StatusOK)
	return
}

func buildTokenInfo(owner *user.User, tkn string, token string, passProtected bool, c gateway.GatewayAPIClient) (TokenInfo, error) {
	t := TokenInfo{Token: tkn}
	if passProtected {
		t.PasswordProtected = true
		return t, nil
	}

	ctx := ctxpkg.ContextSetToken(context.TODO(), token)
	ctx = ctxpkg.ContextSetUser(ctx, owner)
	ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, token)

	sRes, err := getTokenStatInfo(ctx, c, tkn)
	if err != nil || sRes.Status.Code != rpc.Code_CODE_OK {
		return t, fmt.Errorf("can't stat resource. %+v %s", sRes, err)
	}

	ls := &link.PublicShare{}
	_ = json.Unmarshal(sRes.Info.Opaque.Map["link-share"].Value, ls)

	t.StorageID = ls.ResourceId.GetStorageId()
	t.OpaqueID = ls.ResourceId.GetOpaqueId()

	baseURI, ok := ctx.Value(net.CtxKeyBaseURI).(string)
	if ok {
		ref := path.Join(baseURI, sRes.Info.Path)
		if sRes.Info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			ref += "/"
		}
		t.LinkURL = ref
	}

	return t, nil
}

func getInfoForToken(tkn string, q url.Values, c gateway.GatewayAPIClient) (owner *user.User, token string, passwordProtected bool, err error) {
	ctx := context.Background()

	sig := q.Get("signature")
	expiration := q.Get("expiration")
	res, err := handleSignatureAuth(ctx, c, tkn, sig, expiration)
	if err != nil {
		return
	}

	switch res.Status.Code {
	case rpc.Code_CODE_OK:
		// nothing to do
	case rpc.Code_CODE_PERMISSION_DENIED:
		if res.Status.Message != "wrong password" {
			err = errors.New("not found")
			return
		}

		passwordProtected = true
		return
	default:
		err = fmt.Errorf("authentication returned unsupported status code '%d'", res.Status.Code)
		return
	}

	return res.User, res.Token, false, nil
}
