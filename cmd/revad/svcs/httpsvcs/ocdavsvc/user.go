package ocdavsvc

import (
	"encoding/json"
	"net/http"

	"github.com/cernbox/reva/pkg/user"
	"github.com/pkg/errors"
)

func (s *svc) doUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(contextUserRequiredErr("userrequired"), "error getting user from ctx")
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res := &ocsResponse{
		OCS: &ocsPayload{
			Meta: ocsMetaOK,
			Data: &ocsUserData{
				ID:          u.Username,
				DisplayName: u.DisplayName,
				Email:       u.Mail,
			},
		},
	}
	encoded, err := json.Marshal(res)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(encoded)
}

type contextUserRequiredErr string

func (err contextUserRequiredErr) Error() string   { return string(err) }
func (err contextUserRequiredErr) IsUserRequired() {}
