package ocdavsvc

import (
	"encoding/json"
	"net/http"

	"github.com/cernbox/reva/pkg/user"
	"github.com/pkg/errors"
)

func (s *svc) doUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type response struct {
		Data       interface{} `json:"data"`
		Status     string      `json:"status"`
		StatusCode int         `json:"statuscode"`
	}

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(contextUserRequiredErr("userrequired"), "error getting user from ctx")
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	userData := struct {
		ID          string `json:"id"` // TODO needs better naming, clarify if we need a userid, a username or both
		DisplayName string `json:"display-name"`
		Email       string `json:"email"`
	}{ID: u.Username, DisplayName: u.DisplayName, Email: u.Mail}

	meta := &responseMeta{Status: "ok", StatusCode: 100, Message: "OK"}
	payload := &ocsPayload{Meta: meta, Data: userData}
	res := &ocsResponse{OCS: payload}
	encoded, err := json.Marshal(res)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(encoded)
}

type responseMeta struct {
	Status       string `json:"status"`
	StatusCode   int    `json:"statuscode"`
	Message      string `json:"message"`
	TotalItems   string `json:"totalitems"`
	ItemsPerPage string `json:"itemsperpage"`
}

type ocsPayload struct {
	Meta *responseMeta `json:"meta"`
	Data interface{}   `json:"data"`
}

type ocsResponse struct {
	OCS *ocsPayload `json:"ocs"`
}

type contextUserRequiredErr string

func (err contextUserRequiredErr) Error() string   { return string(err) }
func (err contextUserRequiredErr) IsUserRequired() {}
