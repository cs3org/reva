package demo

import (
	"context"
	//"fmt"
	"testing"

	"github.com/cernbox/reva/pkg/token"
)

func TestEncodeDecode(t *testing.T) {
	ctx := context.Background()
	m := New()
	groups := []string{"radium-lovers"}
	claims := token.Claims{
		"username":     "marie",
		"groups":       groups,
		"display_name": "Marie Curie",
		"mail":         "marie@example.org",
	}

	encoded, err := m.ForgeToken(ctx, claims)
	if err != nil {
		t.Fatal(err)
	}

	decodedClaims, err := m.DismantleToken(ctx, encoded)
	if err != nil {
		t.Fatal(err)
	}

	if claims["username"] != decodedClaims["username"] {
		t.Fatalf("username claims differ: expected=%s got=%s", claims["username"], decodedClaims["username"])
	}
	if claims["display_name"] != decodedClaims["display_name"] {
		t.Fatalf("display_name claims differ: expected=%s got=%s", claims["display_name"], decodedClaims["display_name"])
	}
	if claims["mail"] != decodedClaims["mail"] {
		t.Fatalf("mail claims differ: expected=%s got=%s", claims["mail"], decodedClaims["mail"])
	}

	decodedGroups, ok := decodedClaims["groups"].([]string)
	if !ok {
		t.Fatal("groups key in decoded claims is not []string")
	}

	if len(groups) != len(groups) {
		t.Fatalf("groups claims differ in length: expected=%d got=%d", len(groups), len(decodedGroups))
	}
}
