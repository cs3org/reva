package ocdavsvc

import (
	"encoding/json"
	"net/http"
)

func (s *svc) doUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type response struct {
		Data       interface{} `json:"data"`
		Status     string      `json:"status"`
		StatusCode int         `json:"statuscode"`
	}

	userData := struct {
		ID          string `json:"id"`
		DisplayName string `json:"display-name"`
		Email       string `json:"email"`
	}{ID: "einstein", DisplayName: "Mister Einstein", Email: "einstein@relativity.com"}

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
