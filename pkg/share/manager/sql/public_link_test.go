package sql

import (
	"context"
	"log"
	"os"
	"slices"
	"sort"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/permissions"
	publicshare "github.com/cs3org/reva/v3/pkg/publicshare"
)

func setupSuiteLinks(tb testing.TB) (publicshare.Manager, error, func(tb testing.TB)) {
	ctx := context.Background()
	dbName := "test_db.sqlite"
	cfg := map[string]any{
		"db_engine": "sqlite",
		"db_name":   dbName,
	}
	mgr, err := NewPublicShareManager(ctx, cfg)
	if err != nil {
		os.Remove(dbName)
		return nil, err, nil
	}

	// Return a function to teardown the test
	return mgr, nil, func(tb testing.TB) {
		log.Println("teardown suite")
		os.Remove(dbName)
	}
}

func getTestPublicLinkGrant(password string) *link.Grant {
	return &link.Grant{
		Permissions: &link.PublicSharePermissions{
			Permissions: permissions.OcsPermissions(1).AsCS3Permissions(),
		},
		Password: password,
		Expiration: &typespb.Timestamp{
			Seconds: uint64(time.Now().Add(24 * time.Hour).Unix()),
		},
	}
}

func TestCreatePublicShare(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	publicShare, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if publicShare.Token == "" {
		t.Error("Expected public share token to be generated")
	}

	if publicShare.Owner.OpaqueId != user.Id.OpaqueId {
		t.Errorf("Expected owner ID %s, got %s", user.Id.OpaqueId, publicShare.Owner.OpaqueId)
	}
}

func TestCreatePublicShareWithPassword(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	passwordGrant := getTestPublicLinkGrant("secret")

	passwordPublicShare, err := mgr.CreatePublicShare(userctx, nil, file, passwordGrant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	unprotectedPublicShare, err := mgr.CreatePublicShare(userctx, nil, file, getTestPublicLinkGrant(""), "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if !passwordPublicShare.PasswordProtected {
		t.Error("Expected share to be password protected")
	}

	if unprotectedPublicShare.PasswordProtected {
		t.Error("Did not expect unprotected share to be password protected")
	}

	if passwordPublicShare.Token == "" || unprotectedPublicShare.Token == "" {
		t.Error("Expected a token to be generated")
	}
}

func TestGetPublicShareByToken(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	createdFirst, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	createdSecond, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	retrievedFirst, err := mgr.GetPublicShareByToken(userctx, createdFirst.Token, nil, false)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if retrievedFirst.Token != createdFirst.Token {
		t.Errorf("Expected token %s, got %s", createdFirst.Token, retrievedFirst.Token)
	}

	retrievedSecond, err := mgr.GetPublicShareByToken(userctx, createdSecond.Token, nil, false)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if retrievedSecond.Token != createdSecond.Token {
		t.Errorf("Expected token %s, got %s", createdSecond.Token, retrievedSecond.Token)
	}
}

func TestUpdatePublicShare(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	share, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	// Test updating display name
	newName := "Updated Name"
	updateReq := &link.UpdatePublicShareRequest{
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Id{
				Id: share.Id,
			},
		},
		Update: &link.UpdatePublicShareRequest_Update{
			Type:        link.UpdatePublicShareRequest_Update_TYPE_DISPLAYNAME,
			DisplayName: newName,
		},
	}

	updated, err := mgr.UpdatePublicShare(userctx, nil, updateReq, grant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if updated.DisplayName != newName {
		t.Errorf("Expected display name %s, got %s", newName, updated.DisplayName)
	}
}

func TestRevokePublicShare(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	share, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	err = mgr.RevokePublicShare(userctx, nil, &link.PublicShareReference{
		Spec: &link.PublicShareReference_Id{
			Id: share.Id,
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	// Verify share is deleted
	_, err = mgr.GetPublicShare(userctx, nil, &link.PublicShareReference{
		Spec: &link.PublicShareReference_Id{
			Id: share.Id,
		},
	}, false)
	if err == nil {
		t.Error("Expected share to be deleted")
	}
}

func TestListPublicShares(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	_, err = mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	shares, err := mgr.ListPublicShares(userctx, nil, nil, file, false)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 1 {
		t.Errorf("Expected 1 public share, got %d", len(shares))
	}
}

func TestListPublicSharesWithFilters(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	share, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	filters := []*link.ListPublicSharesRequest_Filter{
		{
			Type: link.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID,
			Term: &link.ListPublicSharesRequest_Filter_ResourceId{
				ResourceId: &provider.ResourceId{
					StorageId: file.Id.StorageId,
					OpaqueId:  file.Id.OpaqueId,
				},
			},
		},
	}

	shares, err := mgr.ListPublicShares(userctx, nil, filters, file, false)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 1 {
		t.Errorf("Expected 1 public share, got %d", len(shares))
	}

	if shares[0].Id.OpaqueId != share.Id.OpaqueId {
		t.Errorf("Expected share ID %s, got %s", share.Id.OpaqueId, shares[0].Id.OpaqueId)
	}
}

// TestListPublicShareSpaces asserts that every space holding a public link comes
// back once, and that the links of a batch of spaces come back together. This is
// what the reconciliation jobs walk, so it has to match what ListPublicShares
// would return.
func TestListPublicShareSpaces(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)

	// two links in one space and one in another, so a space that holds more
	// than one link still comes back once.
	for _, s := range []struct{ space, inode string }{
		{"space-a", "200"},
		{"space-a", "201"},
		{"space-b", "202"},
	} {
		file := getRandomFile(user)
		file.Id.SpaceId = s.space
		file.Id.OpaqueId = s.inode

		_, err := mgr.CreatePublicShare(userctx, nil, file, getTestPublicLinkGrant(""), "test", false, false, "")
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
	}

	spaces, err := mgr.(*PublicShareMgr).ListPublicShareSpaces(userctx)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	sort.Strings(spaces)
	if len(spaces) != 2 || spaces[0] != "space-a" || spaces[1] != "space-b" {
		t.Errorf("Expected [space-a space-b], got %v", spaces)
	}

	links, err := mgr.(*PublicShareMgr).ListPublicSharesInSpaces(userctx, spaces)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	if len(links) != 3 {
		t.Errorf("Expected 3 links in both spaces, got %d", len(links))
	}

	links, err = mgr.(*PublicShareMgr).ListPublicSharesInSpaces(userctx, []string{"space-b"})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	if len(links) != 1 || links[0].ResourceId.OpaqueId != "202" {
		t.Errorf("Expected only the link in space-b, got %+v", links)
	}
}

func TestListPublicLinksInSpace(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)
	if err != nil {
		t.Fatal(err)
	}
	pm := mgr.(*PublicShareMgr)

	ids := map[string]*link.PublicShareId{}
	for _, l := range []struct{ name, owner, space string }{
		{"a-alice", "alice", "space-a"},
		{"a-bob", "bob", "space-a"},
		{"a-orphan", "alice", "space-a"},
		{"b-alice", "alice", "space-b"},
	} {
		userctx := getUserContext(l.owner)
		u, _ := appctx.ContextGetUser(userctx)
		file := getRandomFile(u)
		file.Id.SpaceId, file.Id.OpaqueId = l.space, l.name
		created, err := mgr.CreatePublicShare(userctx, u, file, getTestPublicLinkGrant(""), "", false, false, "")
		if err != nil {
			t.Fatal(err)
		}
		ids[l.name] = created.Id
	}
	if err := pm.MarkAsOrphaned(context.Background(), &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: ids["a-orphan"]}}); err != nil {
		t.Fatal(err)
	}

	owner := func(u string) *link.ListPublicSharesRequest_Filter {
		return &link.ListPublicSharesRequest_Filter{Type: link.ListPublicSharesRequest_Filter_TYPE_OWNER, Term: &link.ListPublicSharesRequest_Filter_Owner{Owner: &userpb.UserId{OpaqueId: u}}}
	}
	for _, tc := range []struct {
		filters        []*link.ListPublicSharesRequest_Filter
		includeOrphans bool
		want           []string
	}{
		{nil, false, []string{"a-alice", "a-bob"}},
		{nil, true, []string{"a-alice", "a-bob", "a-orphan"}},
		{[]*link.ListPublicSharesRequest_Filter{owner("alice")}, true, []string{"a-alice", "a-orphan"}},
	} {
		links, err := pm.ListPublicLinksInSpace("space-a", tc.filters, tc.includeOrphans)
		if err != nil {
			t.Fatal(err)
		}
		got := []string{}
		for _, l := range links {
			got = append(got, l.Inode)
		}
		sort.Strings(got)
		if !slices.Equal(got, tc.want) {
			t.Errorf("filters %v, orphans %v: got %v, want %v", tc.filters, tc.includeOrphans, got, tc.want)
		}
	}
	if _, err := pm.ListPublicLinksInSpace("", nil, true); err == nil {
		t.Error("expected an error without a space")
	}
}

func TestMovePublicShareToAnotherSpace(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)
	if err != nil {
		t.Fatal(err)
	}
	pm := mgr.(*PublicShareMgr)

	userctx := getUserContext("owner")
	u, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(u)
	file.Id.SpaceId = "space-a"
	created, err := mgr.CreatePublicShare(userctx, u, file, getTestPublicLinkGrant(""), "", false, false, "")
	if err != nil {
		t.Fatal(err)
	}
	ref := &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: created.Id}}
	if err := pm.MovePublicShare(userctx, ref, &provider.ResourceId{StorageId: "other", OpaqueId: "42", SpaceId: "space-b"}, "newowner"); err != nil {
		t.Fatal(err)
	}
	l, err := pm.GetPublicLink(userctx, ref, false)
	if err != nil {
		t.Fatal(err)
	}
	if l.UIDOwner != "newowner" || l.Instance != "other" || l.Inode != "42" || l.SpaceID != "space-b" {
		t.Fatalf("link not moved: %+v", l)
	}
}
