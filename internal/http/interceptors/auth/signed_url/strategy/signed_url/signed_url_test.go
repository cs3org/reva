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
	want := "https://example.org/archiver?id=file1&OC-Credential=jgeens&id=file2&OC-Date=2026-06-25T09%3A08%3A08.817Z&OC-Expires=1200&OC-Verb=GET"
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
	want := "https://example.org/archiver?path=%2Fone&id=file1&path=%2Ftwo&id=file2&OC-Credential=jgeens&OC-Date=2026-06-25T09%3A08%3A08.817Z&OC-Expires=1200&OC-Verb=GET"
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
	want := "https://example.org/archiver?id=file1&OC-Credential=jgeens&OC-Date=2026-06-25T09%3A08%3A08.817Z&OC-Expires=1200&OC-Verb=GET"
	if got != want {
		t.Fatalf("unexpected URL to sign:\nwant: %s\n got: %s", want, got)
	}
}

func TestBuildUrlToSignUppercasesPathEncoding(t *testing.T) {
	// The signer signs the percent-encoded path; its encoding hex must be
	// normalized to uppercase so a request delivered with lowercase encoding
	// (%24/%3d) still reproduces the signer's uppercase form (%24/%3D).
	const want = "https://example.org/remote.php/dav/spaces/eoshome-j%24F5SW64ZPOVZWK4RPNIXWUZ3FMVXHG%3D%3D%3D/pres.pptx?OC-Credential=jgeens&OC-Date=2026-06-25T09%3A08%3A08.817Z&OC-Expires=1200&OC-Verb=GET"
	query := "?OC-Credential=jgeens&OC-Date=2026-06-25T09%3A08%3A08.817Z&OC-Expires=1200&OC-Verb=GET"
	paths := map[string]string{
		"lowercase-encoded": "eoshome-j%24F5SW64ZPOVZWK4RPNIXWUZ3FMVXHG%3d%3d%3d",
		"uppercase-encoded": "eoshome-j%24F5SW64ZPOVZWK4RPNIXWUZ3FMVXHG%3D%3D%3D",
	}
	for name, seg := range paths {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequest(
				http.MethodGet,
				"https://example.org/remote.php/dav/spaces/"+seg+"/pres.pptx"+query,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}

			got := SignedURLAuthenticator{}.buildUrlToSign(req)
			if got != want {
				t.Fatalf("unexpected URL to sign:\nwant: %s\n got: %s", want, got)
			}
		})
	}
}

func TestBuildUrlToSignNormalizesEncodingCase(t *testing.T) {
	// The OC-Date value arrives with lowercase percent-encoding (%3a); it must be
	// re-encoded to the canonical uppercase form (%3A) before signing.
	req, err := http.NewRequest(
		http.MethodGet,
		"https://example.org/archiver?id=file1&OC-Credential=jgeens&OC-Date=2026-06-25T09%3a08%3a08.817Z&OC-Expires=1200&OC-Verb=GET",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	got := SignedURLAuthenticator{}.buildUrlToSign(req)
	want := "https://example.org/archiver?id=file1&OC-Credential=jgeens&OC-Date=2026-06-25T09%3A08%3A08.817Z&OC-Expires=1200&OC-Verb=GET"
	if got != want {
		t.Fatalf("unexpected URL to sign:\nwant: %s\n got: %s", want, got)
	}
}
