package upload

import (
	"context"
	"os"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/owncloud/reva/v2/pkg/errtypes"
	"github.com/owncloud/reva/v2/pkg/storage/cache"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/aspects"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/metadata"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/timemanager"
	"github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/tree"
	"github.com/rs/zerolog"
)

// TestInitNewNode calls greetings.initNewNode
func TestInitNewNode(t *testing.T) {
	log := &zerolog.Logger{}
	root := t.TempDir()

	lookup := lookup.New(metadata.NewMessagePackBackend(root, cache.Config{}), &options.Options{Root: root}, &timemanager.Manager{})
	tp := tree.New(lookup, nil, &options.Options{}, nil, log)

	aspects := aspects.Aspects{
		Lookup: lookup,
		Tree:   tp,
	}
	store := NewSessionStore(nil, aspects, root, false, options.TokenOptions{}, log)

	rootNode := node.New("e48c4e7a-beac-4b82-b991-a5cff7b8c39c", "e48c4e7a-beac-4b82-b991-a5cff7b8c39c", "", "", 0, "", providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER, &userv1beta1.UserId{}, lookup)
	rootNode.Exists = true
	rootNode.SpaceRoot = rootNode

	err := os.MkdirAll(rootNode.InternalPath(), 0700)
	if err != nil {
		t.Fatal(err.Error())
	}
	n := node.New("e48c4e7a-beac-4b82-b991-a5cff7b8c39c", "930b7a2e-b745-41e1-8a9b-712582021842", "e48c4e7a-beac-4b82-b991-a5cff7b8c39c", "newchild", 10, "26493c53-2634-45f8-949f-dc07b88df9b0", providerv1beta1.ResourceType_RESOURCE_TYPE_FILE, &userv1beta1.UserId{}, lookup)
	n.SpaceRoot = rootNode
	unlock, err := store.tp.InitNewNode(context.Background(), n, 10)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer func() {
		_ = unlock()
	}()

	// try initializing the same new node again in case a concurrent requests tries to create a file with the same name
	n = node.New("e48c4e7a-beac-4b82-b991-a5cff7b8c39c", "a6ede986-cfcd-41c5-a820-6eee955a1c2b", "e48c4e7a-beac-4b82-b991-a5cff7b8c39c", "newchild", 10, "26493c53-2634-45f8-949f-dc07b88df9b0", providerv1beta1.ResourceType_RESOURCE_TYPE_FILE, &userv1beta1.UserId{}, lookup)
	n.SpaceRoot = rootNode
	unlock2, err := store.tp.InitNewNode(context.Background(), n, 10)
	if _, ok := err.(errtypes.IsAlreadyExists); !ok {
		t.Fatalf(`initNewNode(with same 'newchild' name), %v, want %v`, err, errtypes.AlreadyExists("newchild"))
	}
	_ = unlock2()
}
