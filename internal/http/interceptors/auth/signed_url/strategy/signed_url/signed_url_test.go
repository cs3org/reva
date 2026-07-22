package basic

import (
	"net/http"
	"testing"
)

func TestBuildUrlToSignPreservesSignedParameterOrder(t *testing.T) {
	req, err := http.NewRequest(
		http.MethodGet,
		"https://example.org/archiver?id=file1&OC-Credential=jgeens&id=file2&OC-Date=2026-06-25T09%3A08%3A08.817Z&OC-Expires=1200&OC-Verb=GET",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	got := SignedURLAuthenticator{}.buildUrlToSign(req)
	want := "https://example.org/archiver?id=file1&OC-Credential=jgeens&id=file2&OC-Date=2026-06-25t09%3a08%3a08.817z&OC-Expires=1200&OC-Verb=get"
	if got != want {
		t.Fatalf("unexpected URL to sign:\nwant: %s\n got: %s", want, got)
	}
}

func TestBuildUrlToSignPreservesRepeatedResourceParameters(t *testing.T) {
	req, err := http.NewRequest(
		http.MethodGet,
		"https://example.org/archiver?path=%2Fone&id=file1&path=%2Ftwo&id=file2&OC-Credential=jgeens&OC-Date=2026-06-25T09%3A08%3A08.817Z&OC-Expires=1200&OC-Verb=GET",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	got := SignedURLAuthenticator{}.buildUrlToSign(req)
	want := "https://example.org/archiver?path=%2fone&id=file1&path=%2ftwo&id=file2&OC-Credential=jgeens&OC-Date=2026-06-25t09%3a08%3a08.817z&OC-Expires=1200&OC-Verb=get"
	if got != want {
		t.Fatalf("unexpected URL to sign:\nwant: %s\n got: %s", want, got)
	}
}

func TestBuildUrlToSignFiltersUnsignedParameters(t *testing.T) {
	req, err := http.NewRequest(
		http.MethodGet,
		"https://example.org/archiver?id=file1&OC-Algo=PBKDF2%2F10000-SHA512&OC-Credential=jgeens&foo=bar&OC-Date=2026-06-25T09%3A08%3A08.817Z&OC-Signature=abc&OC-Expires=1200&OC-Verb=GET",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	got := SignedURLAuthenticator{}.buildUrlToSign(req)
	want := "https://example.org/archiver?id=file1&OC-Credential=jgeens&OC-Date=2026-06-25t09%3a08%3a08.817z&OC-Expires=1200&OC-Verb=get"
	if got != want {
		t.Fatalf("unexpected URL to sign:\nwant: %s\n got: %s", want, got)
	}
}
