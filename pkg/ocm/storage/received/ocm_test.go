package ocm

import (
	"encoding/xml"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/studio-b12/gowebdav"
)

// propsWithLockdiscovery builds a gowebdav.Props the way a PROPFIND response
// would. A nil raw omits the key, which is the unlocked case.
func propsWithLockdiscovery(t *testing.T, raw *string) gowebdav.Props {
	t.Helper()

	body := `<?xml version="1.0"?>
<d:prop xmlns:d="DAV:">
  <d:getetag>"abc"</d:getetag>
`
	if raw != nil {
		body += "  <d:lockdiscovery>" + *raw + "</d:lockdiscovery>\n"
	}
	body += "</d:prop>"

	var props gowebdav.Props
	if err := xml.Unmarshal([]byte(body), &props); err != nil {
		t.Fatalf("could not build props: %v", err)
	}
	return props
}

func strptr(s string) *string { return &s }

func TestExtractLock(t *testing.T) {
	// as emitted by ocdav's activeLocks
	exclusive := `<d:activelock>` +
		`<d:locktype><d:write/></d:locktype>` +
		`<d:lockscope><d:exclusive/></d:lockscope>` +
		`<d:depth>Infinity</d:depth>` +
		`<d:owner>some-user@https://localhost:9200</d:owner>` +
		`<d:timeout>Second-1800</d:timeout>` +
		`<d:locktoken><d:href>opaque-lock-token</d:href></d:locktoken>` +
		`</d:activelock>`

	shared := `<d:activelock>` +
		`<d:lockscope><d:shared/></d:lockscope>` +
		`<d:timeout>Infinity</d:timeout>` +
		`<d:locktoken><d:href>shared-token</d:href></d:locktoken>` +
		`</d:activelock>`

	// activeLocks omits d:locktoken when the lock carries no id
	noToken := `<d:activelock>` +
		`<d:lockscope><d:exclusive/></d:lockscope>` +
		`<d:timeout>Infinity</d:timeout>` +
		`</d:activelock>`

	tests := []struct {
		name     string
		raw      *string
		wantID   string
		wantType provider.LockType
	}{
		{
			name:     "exclusive lock yields its token",
			raw:      strptr(exclusive),
			wantID:   "opaque-lock-token",
			wantType: provider.LockType_LOCK_TYPE_EXCL,
		},
		{
			name:     "shared lock keeps its scope",
			raw:      strptr(shared),
			wantID:   "shared-token",
			wantType: provider.LockType_LOCK_TYPE_SHARED,
		},
		{
			// the regression that made every unlocked OCM file read as locked
			name:   "absent lockdiscovery is not a lock",
			raw:    nil,
			wantID: "",
		},
		{
			name:   "lock without a token is not usable",
			raw:    strptr(noToken),
			wantID: "",
		},
		{
			name:   "empty lockdiscovery is not a lock",
			raw:    strptr(""),
			wantID: "",
		},
		{
			name:   "unrelated content is not a lock",
			raw:    strptr("garbage"),
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLock(propsWithLockdiscovery(t, tt.raw))

			if tt.wantID == "" {
				if got != nil {
					t.Fatalf("expected no lock, got %+v", got)
				}
				return
			}

			if got == nil {
				t.Fatal("expected a lock, got nil")
			}
			if got.LockId != tt.wantID {
				t.Errorf("LockId = %q, want %q", got.LockId, tt.wantID)
			}
			if got.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", got.Type, tt.wantType)
			}
		})
	}
}

// The bug this replaced: gowebdav renders a missing prop as "<nil>", so a guard
// on emptiness never fires and callers read an unlocked file as locked.
func TestExtractLockAbsentPropIsNotEmptyString(t *testing.T) {
	props := propsWithLockdiscovery(t, nil)

	if raw := props.GetString(xml.Name{Space: "DAV:", Local: "lockdiscovery"}); raw != "<nil>" {
		t.Skipf("gowebdav no longer renders absent props as <nil> (got %q); extractLock can be simplified", raw)
	}
	if got := extractLock(props); got != nil {
		t.Fatalf("expected no lock for absent lockdiscovery, got %+v", got)
	}
}
