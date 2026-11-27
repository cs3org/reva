package eosgrpc

import (
	"context"
	"syscall"
	"testing"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"google.golang.org/grpc"
)

// execClient keeps the request of Exec, and answers with resp.
type execClient struct {
	erpc.EosClient
	req  *erpc.NSRequest
	resp *erpc.NSResponse
}

func (c *execClient) Exec(ctx context.Context, in *erpc.NSRequest, opts ...grpc.CallOption) (*erpc.NSResponse, error) {
	c.req = in
	return c.resp, nil
}

func TestRestoreDeletedEntryReportsACollision(t *testing.T) {
	cl := &execClient{resp: &erpc.NSResponse{Recycle: &erpc.NSResponse_RecycleResponse{Code: int64(syscall.EEXIST)}}}
	c := &Client{opt: &Options{}, cl: cl}

	err := c.RestoreDeletedEntry(context.Background(), eosclient.Authorization{}, "1a2b3c", false)

	if _, ok := err.(errtypes.IsAlreadyExists); !ok {
		t.Errorf("got error %v, want AlreadyExists", err)
	}
	if cl.req.GetRecycle().GetRestore().GetForceOrigName() {
		t.Error("got a forced restore, want a normal one")
	}
}

func TestRestoreDeletedEntryReportsAnEOSError(t *testing.T) {
	cl := &execClient{resp: &erpc.NSResponse{Recycle: &erpc.NSResponse_RecycleResponse{Code: int64(syscall.EPERM), Msg: "error: not allowed"}}}
	c := &Client{opt: &Options{}, cl: cl}

	if err := c.RestoreDeletedEntry(context.Background(), eosclient.Authorization{}, "1a2b3c", false); err == nil {
		t.Error("got no error, want the error of EOS")
	}
}

func TestRestoreDeletedEntryForcesTheOriginalName(t *testing.T) {
	cl := &execClient{resp: &erpc.NSResponse{}}
	c := &Client{opt: &Options{}, cl: cl}

	if err := c.RestoreDeletedEntry(context.Background(), eosclient.Authorization{}, "1a2b3c", true); err != nil {
		t.Fatal(err)
	}
	if restore := cl.req.GetRecycle().GetRestore(); restore.GetKey() != "1a2b3c" || !restore.GetForceOrigName() {
		t.Errorf("got restore %v, want key 1a2b3c with ForceOrigName", restore)
	}
}
