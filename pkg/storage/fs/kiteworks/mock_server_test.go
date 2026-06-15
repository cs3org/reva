package kiteworks_test

import (
	"net/http"
	"strings"
)

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func mockKiteworksHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/rest/folders/top", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"data":[{"id":"space-1","type":"d","name":"My Docs","path":"/My Docs","modified":"2024-01-01T00:00:00+0000"}]}`)
	})
	mux.HandleFunc("/rest/folders/space-1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"id":"space-1","type":"d","name":"My Docs","path":"/My Docs","modified":"2024-01-01T00:00:00+0000"}`)
	})
	mux.HandleFunc("/rest/folders/space-1/children", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"data":[{"id":"file-1","type":"f","name":"hello.txt","path":"/My Docs/hello.txt","size":14,"modified":"2024-01-01T00:00:00+0000"}]}`)
	})
	mux.HandleFunc("/rest/files/file-1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"id":"file-1","type":"f","name":"hello.txt","path":"/My Docs/hello.txt","size":14,"modified":"2024-01-01T00:00:00+0000"}`)
	})
	mux.HandleFunc("/rest/files/file-1/content", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello kiteworks"))
	})
	mux.HandleFunc("/rest/users/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"id":"user-1","name":"Test User","email":"test@example.com"}`)
	})
	mux.HandleFunc("/rest/quotas", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"folder_quota_allowed":1073741824,"folder_quota_used":14}`)
	})
	serverError := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server error"}`))
	}
	mux.HandleFunc("/rest/folders/error-500", serverError)
	mux.HandleFunc("/rest/files/error-500", serverError)
	mux.HandleFunc("/rest/folders/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/folders/")
		id = strings.Split(id, "/")[0]
		http.Error(w, `{"error":"not found","id":"`+id+`"}`, http.StatusNotFound)
	})

	return mux
}

