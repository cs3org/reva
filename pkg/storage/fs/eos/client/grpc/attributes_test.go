package eosgrpc

import (
	"testing"

	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
)

// The attribute map a FileInfo carries keeps the "sys." prefix and strips
// "user.", so a user attribute arrives under its bare name and only a key that
// starts with a type EOS knows may be split.
func TestGetAttribute(t *testing.T) {
	tests := []struct {
		key      string
		wantType eosclient.AttrType
		wantKey  string
	}{
		{key: "sys.acl", wantType: eosclient.SystemAttr, wantKey: "acl"},
		{key: "sys.forced.checksum", wantType: eosclient.SystemAttr, wantKey: "forced.checksum"},
		{key: "reva.lockpayload", wantType: eosclient.UserAttr, wantKey: "reva.lockpayload"},
		{key: "iversion", wantType: eosclient.UserAttr, wantKey: "iversion"},
	}

	for _, tt := range tests {
		attr := getAttribute(tt.key, "value")
		if attr.Type != tt.wantType {
			t.Errorf("getAttribute(%q) type = %v, want %v", tt.key, attr.Type, tt.wantType)
		}
		if attr.Key != tt.wantKey {
			t.Errorf("getAttribute(%q) key = %q, want %q", tt.key, attr.Key, tt.wantKey)
		}
		if attr.Val != "value" {
			t.Errorf("getAttribute(%q) val = %q, want %q", tt.key, attr.Val, "value")
		}
	}
}
