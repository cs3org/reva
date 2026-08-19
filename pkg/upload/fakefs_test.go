package upload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/gomega"
	tusd "github.com/tus/tusd/v2/pkg/handler"
	eventsapi "go-micro.dev/v4/events"

	"github.com/owncloud/reva/v2/pkg/errtypes"
	"github.com/owncloud/reva/v2/pkg/events"
	"github.com/owncloud/reva/v2/pkg/storage"
)

// fakeFS is a storage.FS test double recording the driver calls in order.
type fakeFS struct {
	storage.FS

	// calls is the ordered driver calls, with the arguments the specs assert on.
	calls []string

	// canned returns
	md    *provider.ResourceInfo
	mdErr error
	tree  map[string]*provider.ResourceInfo
	// mdErrs fails one path in the tree, for the errors that are not not-found.
	mdErrs       map[string]error
	quota        uint64
	quotaErr     error
	lock         *provider.Lock
	lockErr      error
	pathByID     string
	pathByIDErr  error
	touched      *provider.ResourceId
	touchedOwner *userpb.UserId
	touchErr     error
	markErr      error
	prepared     *storage.PrepareUploadResult
	prepareErr   error
	commitErr    error
	rollbackErr  error
	deleteErr    error

	// markErrAfter applies markErr only from the nth MarkProcessing call on.
	markErrAfter int
	markCalls    int

	// what the driver was handed, for the arguments the call list does not carry.
	committed   storage.UploadSource
	prepareInfo storage.UploadInfo
	rolledBack  storage.RollbackInfo
	touchRef    *provider.Reference
	touchMtime  string

	// hooks fire inside a driver call, to make the coordinator's next step fail.
	afterMark, afterPrepare func()
}

func (f *fakeFS) record(format string, args ...interface{}) {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

// GetMD answers from tree when the spec set one up, otherwise from md.
func (f *fakeFS) GetMD(_ context.Context, ref *provider.Reference, _, _ []string) (*provider.ResourceInfo, error) {
	f.record("GetMD(%s)", ref.GetPath())
	if err, ok := f.mdErrs[ref.GetPath()]; ok {
		return nil, err
	}
	if f.tree != nil {
		if ri, ok := f.tree[ref.GetPath()]; ok {
			return ri, nil
		}
		return nil, errtypes.NotFound(ref.GetPath())
	}
	if f.mdErr != nil {
		return nil, f.mdErr
	}
	return f.md, nil
}

func (f *fakeFS) GetQuota(_ context.Context, _ *provider.Reference) (uint64, uint64, uint64, error) {
	f.record("GetQuota")
	return 0, 0, f.quota, f.quotaErr
}

func (f *fakeFS) GetLock(_ context.Context, _ *provider.Reference) (*provider.Lock, error) {
	f.record("GetLock")
	return f.lock, f.lockErr
}

func (f *fakeFS) GetPathByID(_ context.Context, _ *provider.ResourceId) (string, error) {
	f.record("GetPathByID")
	return f.pathByID, f.pathByIDErr
}

func (f *fakeFS) TouchFile(_ context.Context, ref *provider.Reference, markprocessing bool, mtime string) (*storage.TouchFileResult, error) {
	f.record("TouchFile(markprocessing=%v)", markprocessing)
	f.touchRef, f.touchMtime = ref, mtime
	if f.touchErr != nil {
		return nil, f.touchErr
	}
	return &storage.TouchFileResult{
		ResourceID: f.touched,
		SpaceID:    f.touched.GetSpaceId(),
		SpaceOwner: f.touchedOwner,
	}, nil
}

func (f *fakeFS) MarkProcessing(_ context.Context, _ *provider.Reference, processing bool, _ string) error {
	f.record("MarkProcessing(%v)", processing)
	f.markCalls++
	if f.markErr != nil && f.markCalls > f.markErrAfter {
		return f.markErr
	}
	if f.afterMark != nil {
		f.afterMark()
	}
	return nil
}

func (f *fakeFS) PrepareUpload(_ context.Context, _ *provider.Reference, _ string, info storage.UploadInfo) (*storage.PrepareUploadResult, error) {
	f.record("PrepareUpload(size=%d)", info.Size)
	f.prepareInfo = info
	if f.prepareErr != nil {
		return nil, f.prepareErr
	}
	if f.afterPrepare != nil {
		f.afterPrepare()
	}
	if f.prepared != nil {
		return f.prepared, nil
	}
	return &storage.PrepareUploadResult{}, nil
}

func (f *fakeFS) CommitUpload(_ context.Context, _ *provider.Reference, _ string, source storage.UploadSource) error {
	f.record("CommitUpload(length=%d)", source.Length)
	f.committed = source
	return f.commitErr
}

func (f *fakeFS) RollbackUpload(_ context.Context, _ *provider.Reference, _ string, info storage.RollbackInfo) error {
	f.record("RollbackUpload(nodeExisted=%v,sizeDiff=%d)", info.NodeExisted, info.SizeDiff)
	f.rolledBack = info
	return f.rollbackErr
}

func (f *fakeFS) Delete(_ context.Context, _ *provider.Reference) (*storage.DeleteResult, error) {
	f.record("Delete")
	return nil, f.deleteErr
}

// fakePublisher is an events.Publisher test double keeping the published events.
type fakePublisher struct {
	published []interface{}
	err       error
}

func (p *fakePublisher) Publish(_ string, event interface{}, _ ...eventsapi.PublishOption) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, event)
	return nil
}

// channelPublisher hands each published event to the spec's goroutine, which is how
// the consumer specs observe a background commit without racing on a slice.
type channelPublisher struct {
	published chan interface{}
}

func (p *channelPublisher) Publish(_ string, event interface{}, _ ...eventsapi.PublishOption) error {
	p.published <- event
	return nil
}

// fakeConsumer is an events.Consumer test double handing out a channel the specs
// push raw stream events into.
type fakeConsumer struct {
	ch  chan eventsapi.Event
	err error

	// group is what events.Consume subscribed with.
	group string
}

func (c *fakeConsumer) Consume(_ string, opts ...eventsapi.ConsumeOption) (<-chan eventsapi.Event, error) {
	if c.err != nil {
		return nil, c.err
	}
	o := &eventsapi.ConsumeOptions{}
	for _, opt := range opts {
		opt(o)
	}
	c.group = o.Group
	return c.ch, nil
}

// pushEvent delivers an event the way the stream does: a JSON payload plus the type
// metadata events.Consume dispatches on.
func pushEvent(ch chan eventsapi.Event, ev interface{}) {
	payload, err := json.Marshal(ev)
	Expect(err).ToNot(HaveOccurred())
	ch <- eventsapi.Event{
		Metadata: map[string]string{events.MetadatakeyEventType: reflect.TypeOf(ev).String()},
		Payload:  payload,
	}
}

// brokenSession fails what a real FileSession cannot be made to fail from outside.
type brokenSession struct {
	Session

	// failPersistAfter is how many Persist calls succeed before they start failing.
	failPersist      bool
	failPersistAfter int
	persistCalls     int

	urlErr     error
	getInfoErr error
	readErr    error
}

// GetReader hands back a reader that fails on read, which a missing file cannot.
func (s *brokenSession) GetReader(ctx context.Context) (io.ReadCloser, error) {
	if s.readErr != nil {
		return io.NopCloser(failingReader{err: s.readErr}), nil
	}
	return s.Session.GetReader(ctx)
}

func (s *brokenSession) Persist(ctx context.Context) error {
	s.persistCalls++
	if s.failPersist && s.persistCalls > s.failPersistAfter {
		return errors.New("no space left on device")
	}
	return s.Session.Persist(ctx)
}

func (s *brokenSession) GetInfo(ctx context.Context) (tusd.FileInfo, error) {
	if s.getInfoErr != nil {
		return tusd.FileInfo{}, s.getInfoErr
	}
	return s.Session.GetInfo(ctx)
}

func (s *brokenSession) URL(ctx context.Context) (string, error) {
	if s.urlErr != nil {
		return "", s.urlErr
	}
	return s.Session.URL(ctx)
}

// brokenStore stands in for the store where the code creates or loads its own
// session.
type brokenStore struct {
	SessionStore
	failPersist bool
	listErr     error

	// loadedURLErr fails URL() on the sessions Get hands back.
	loadedURLErr error
}

func (s *brokenStore) New(ctx context.Context) Session {
	return &brokenSession{Session: s.SessionStore.New(ctx), failPersist: s.failPersist}
}

func (s *brokenStore) Get(ctx context.Context, id string) (Session, error) {
	session, err := s.SessionStore.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &brokenSession{
		Session:     session,
		failPersist: s.failPersist,
		urlErr:      s.loadedURLErr,
	}, nil
}

func (s *brokenStore) List(ctx context.Context) ([]Session, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.SessionStore.List(ctx)
}
