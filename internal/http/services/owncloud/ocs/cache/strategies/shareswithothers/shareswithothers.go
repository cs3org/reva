package shareswithothers

import (
	"net/http"
	"net/http/httptest"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/cache"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/sharing/shares"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
)

type strategy struct {
	handler *shares.Handler
}

func New(h *shares.Handler) cache.Warmuper {
	return &strategy{
		handler: h,
	}
}

func (s *strategy) Warmup(r *http.Request) (string, cache.ActionFunc) {
	user := ctxpkg.ContextMustGetUser(r.Context())

	return user.Id.OpaqueId, func() {
		w := httptest.NewRecorder()
		s.handler.ListSharesWithOthers(w, r)
	}
}
