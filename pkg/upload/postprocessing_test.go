package upload

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	eventsapi "go-micro.dev/v4/events"

	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/errtypes"
	"github.com/owncloud/reva/v2/pkg/events"
	"github.com/owncloud/reva/v2/pkg/rhttp/datatx/metrics"
	"github.com/owncloud/reva/v2/pkg/storage"
)

// The second half of an async upload: finishUpload staged the bytes and published
// BytesReceived, and postprocessing now reports back what to do with them.
var _ = Describe("the postprocessing consumer", func() {
	var (
		ctx   context.Context
		store *FileStore
		fs    *fakeFS
		pub   *fakePublisher
		c     *coordinator
	)

	// preparedSession is an upload waiting on a verdict: node flagged, bytes staged.
	preparedSession := func(nodeExists bool) Session {
		session := stagedSession(ctx, store, nodeExists)
		session.SetSizeDiff(bodyLen)
		Expect(session.Persist(ctx)).To(Succeed())
		return session
	}

	BeforeEach(func() {
		ctx = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id:       &userpb.UserId{OpaqueId: "alice", Idp: "idp.example.com"},
			Username: "alice",
		})
		store = NewFileStore(filepath.Join(GinkgoT().TempDir(), "uploads"), TokenOptions{
			DataGatewayEndpoint:  "https://cloud.example.com/data",
			DownloadEndpoint:     "https://cloud.example.com/data/",
			TransferSharedSecret: "secret",
			TransferExpires:      3600,
		}, nopLog())
		Expect(store.Setup()).To(Succeed())

		fs = &fakeFS{
			touched: &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
			md: &provider.ResourceInfo{
				Id:   &provider.ResourceId{StorageId: mountID, SpaceId: spaceID, OpaqueId: nodeID},
				Etag: "etag-after-commit",
			},
		}
		pub = &fakePublisher{}
		c = NewCoordinator(fs, store, "", pub)
	})

	Describe("StartPostprocessing", func() {
		It("subscribes and switches the coordinator over to async uploads", func() {
			stream := &fakeConsumer{ch: make(chan eventsapi.Event)}

			Expect(c.StartPostprocessing(stream, "storage-users", mountID, 1)).To(Succeed())

			Expect(c.async).To(BeTrue())
			Expect(c.mountID).To(Equal(mountID))
			Expect(stream.group).To(Equal("storage-users"))
		})

		// Deferring a commit is only safe if something will arrive to finish it.
		It("refuses to enable async uploads without a publisher", func() {
			c = NewCoordinator(fs, store, "", nil)

			err := c.StartPostprocessing(&fakeConsumer{ch: make(chan eventsapi.Event)}, "storage-users", mountID, 1)

			Expect(err).To(MatchError(ContainSubstring("need an event publisher")))
			Expect(c.async).To(BeFalse())
		})

		// Uploads would wait for a verdict that never comes.
		It("leaves the coordinator synchronous when it cannot subscribe", func() {
			stream := &fakeConsumer{err: errors.New("nats unreachable")}

			Expect(c.StartPostprocessing(stream, "storage-users", mountID, 1)).To(MatchError("nats unreachable"))
			Expect(c.async).To(BeFalse())
		})

		It("runs a single consumer when none were configured", func() {
			published := make(chan interface{}, 1)
			c = NewCoordinator(fs, store, "", &channelPublisher{published: published})
			stream := &fakeConsumer{ch: make(chan eventsapi.Event)}

			Expect(c.StartPostprocessing(stream, "storage-users", mountID, 0)).To(Succeed())

			// The one consumer is what drains the channel.
			pushEvent(stream.ch, events.RestartPostprocessing{UploadID: preparedSession(false).ID()})
			Eventually(published).Should(Receive(BeAssignableToTypeOf(events.BytesReceived{})))
		})
	})

	// The consumer goroutine reaches the same handlers the specs below call directly.
	Describe("Postprocessing", func() {
		It("commits an upload postprocessing cleared", func() {
			published := make(chan interface{}, 1)
			c = NewCoordinator(fs, store, "", &channelPublisher{published: published})
			stream := &fakeConsumer{ch: make(chan eventsapi.Event)}
			Expect(c.StartPostprocessing(stream, "storage-users", mountID, 1)).To(Succeed())
			session := preparedSession(false)

			pushEvent(stream.ch, events.PostprocessingFinished{
				UploadID: session.ID(),
				Outcome:  events.PPOutcomeContinue,
			})

			var ready events.UploadReady
			Eventually(published).Should(Receive(&ready))
			Expect(ready.Failed).To(BeFalse())
			Expect(ready.UploadID).To(Equal(session.ID()))
		})

		It("stops when the stream closes", func() {
			ch := make(chan events.Event)
			done := make(chan struct{})
			go func() { c.Postprocessing(ch); close(done) }()

			close(ch)

			Eventually(done).Should(BeClosed())
		})

		// An event type the coordinator did not register never reaches a handler.
		It("ignores an event it does not subscribe to", func() {
			published := make(chan interface{}, 2)
			c = NewCoordinator(fs, store, "", &channelPublisher{published: published})
			stream := &fakeConsumer{ch: make(chan eventsapi.Event)}
			Expect(c.StartPostprocessing(stream, "storage-users", mountID, 1)).To(Succeed())
			session := preparedSession(false)

			pushEvent(stream.ch, events.PostprocessingRetry{UploadID: session.ID()})
			// A registered event behind it proves the first one was consumed and dropped.
			pushEvent(stream.ch, events.RestartPostprocessing{UploadID: session.ID()})

			Eventually(published).Should(Receive(BeAssignableToTypeOf(events.BytesReceived{})))
			Consistently(published).ShouldNot(Receive())
		})
	})

	// Postprocessing broadcasts its results to every storage provider, so each has
	// to recognise its own.
	Describe("servesStorage", func() {
		BeforeEach(func() {
			c.mountID = mountID
		})

		It("accepts an event for the storage it serves", func() {
			Expect(c.servesStorage(&provider.ResourceId{StorageId: mountID})).To(BeTrue())
		})

		It("drops an event for another storage", func() {
			Expect(c.servesStorage(&provider.ResourceId{StorageId: "storage-2"})).To(BeFalse())
		})

		// Events that name no storage predate this and are accepted.
		It("accepts an event that names no storage", func() {
			Expect(c.servesStorage(&provider.ResourceId{})).To(BeTrue())
			Expect(c.servesStorage(nil)).To(BeTrue())
		})

		// A single provider in tests sees a private stream.
		It("accepts everything when it serves no particular storage", func() {
			c.mountID = ""
			Expect(c.servesStorage(&provider.ResourceId{StorageId: "storage-2"})).To(BeTrue())
		})

		It("drops a finished event for another storage before loading the session", func() {
			session := preparedSession(false)

			c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
				UploadID:   session.ID(),
				ResourceID: &provider.ResourceId{StorageId: "storage-2"},
				Outcome:    events.PPOutcomeContinue,
			}})

			Expect(fs.calls).To(BeEmpty())
		})

		It("drops a step event for another storage before loading the session", func() {
			session := preparedSession(false)

			c.processEvent(ctx, events.Event{Event: events.PostprocessingStepFinished{
				UploadID:     session.ID(),
				ResourceID:   &provider.ResourceId{StorageId: "storage-2"},
				FinishedStep: events.PPStepAntivirus,
				Result:       events.VirusscanResult{Description: "clean"},
			}})

			reloaded, err := store.Get(ctx, session.ID())
			Expect(err).ToNot(HaveOccurred())
			result, _ := reloaded.ScanData()
			Expect(result).To(BeEmpty())
		})
	})

	Describe("PostprocessingFinished", func() {
		Context("continue", func() {
			It("commits the staged bytes and retires the session", func() {
				session := preparedSession(false)

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PPOutcomeContinue,
				}})

				Expect(fs.calls).To(Equal([]string{
					"CommitUpload(length=17)",
					"MarkProcessing(false)",
					"GetMD()",
				}))
				_, err := store.Get(ctx, session.ID())
				Expect(err).To(HaveOccurred())
				_, sErr := os.Stat(session.BinPath())
				Expect(errors.Is(sErr, os.ErrNotExist)).To(BeTrue())
			})

			It("announces the file as available", func() {
				session := preparedSession(false)

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PPOutcomeContinue,
				}})

				Expect(pub.published).To(HaveLen(1))
				ready := pub.published[0].(events.UploadReady)
				Expect(ready.Failed).To(BeFalse())
				Expect(ready.UploadID).To(Equal(session.ID()))
				Expect(ready.Filename).To(Equal("report.docx"))
			})

			// The verdict rides along with the bytes.
			It("commits under the verdict the scan recorded", func() {
				session := preparedSession(false)
				session.SetScanData("clean", time.Unix(1700000000, 0))
				Expect(session.Persist(ctx)).To(Succeed())

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PPOutcomeContinue,
				}})

				Expect(fs.committed.ScanResult).To(Equal("clean"))
				Expect(fs.committed.ScanDate).To(BeTemporally("==", time.Unix(1700000000, 0)))
			})

			// Nobody is waiting on the response, so the bytes are kept for a retry
			// with RestartPostprocessing rather than silently lost.
			It("keeps the node marked and the session on disk when the commit fails", func() {
				session := preparedSession(false)
				fs.commitErr = errors.New("blobstore unavailable")

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PPOutcomeContinue,
				}})

				Expect(fs.calls).ToNot(ContainElement("MarkProcessing(false)"))
				Expect(fs.calls).ToNot(ContainElement(ContainSubstring("RollbackUpload")))
				_, err := store.Get(ctx, session.ID())
				Expect(err).ToNot(HaveOccurred())
				_, sErr := os.Stat(session.BinPath())
				Expect(sErr).ToNot(HaveOccurred())
			})

			// Clients wait on UploadReady, so staying silent would leave them hanging.
			It("reports the failure to the client when the commit fails", func() {
				session := preparedSession(false)
				fs.commitErr = errors.New("blobstore unavailable")

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PPOutcomeContinue,
				}})

				Expect(pub.published).To(HaveLen(1))
				Expect(pub.published[0].(events.UploadReady).Failed).To(BeTrue())
			})
		})

		// An abort is transient, so the bytes stay for a restart.
		Context("abort", func() {
			It("reverts the node but keeps the bytes and the session", func() {
				session := preparedSession(true)

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PPOutcomeAbort,
				}})

				Expect(fs.calls).To(Equal([]string{
					"RollbackUpload(nodeExisted=true,sizeDiff=17)",
					"MarkProcessing(false)",
				}))
				_, err := store.Get(ctx, session.ID())
				Expect(err).ToNot(HaveOccurred())
				_, sErr := os.Stat(session.BinPath())
				Expect(sErr).ToNot(HaveOccurred())
			})

			It("tells the client the upload failed", func() {
				session := preparedSession(false)

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PPOutcomeAbort,
				}})

				Expect(pub.published).To(HaveLen(1))
				Expect(pub.published[0].(events.UploadReady).Failed).To(BeTrue())
			})

			It("counts the abort", func() {
				before := testutil.ToFloat64(metrics.UploadSessionsAborted)

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: preparedSession(false).ID(),
					Outcome:  events.PPOutcomeAbort,
				}})

				Expect(testutil.ToFloat64(metrics.UploadSessionsAborted) - before).To(Equal(float64(1)))
			})

			// The rollback already purged a node this upload created.
			It("tolerates a node the rollback removed", func() {
				session := preparedSession(false)
				fs.markErr = errtypes.NotFound("no such node")

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PPOutcomeAbort,
				}})

				Expect(pub.published).To(HaveLen(1))
			})

			It("still unmarks when the rollback fails", func() {
				session := preparedSession(false)
				fs.rollbackErr = errors.New("no such node")

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PPOutcomeAbort,
				}})

				Expect(fs.calls).To(ContainElement("MarkProcessing(false)"))
			})
		})

		// A virus, or an upload an admin discarded: nothing is kept.
		Context("delete", func() {
			It("reverts the node and discards the bytes and the session", func() {
				session := preparedSession(true)

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PPOutcomeDelete,
				}})

				Expect(fs.calls).To(Equal([]string{
					"RollbackUpload(nodeExisted=true,sizeDiff=17)",
					"MarkProcessing(false)",
				}))
				_, err := store.Get(ctx, session.ID())
				Expect(err).To(HaveOccurred())
				_, sErr := os.Stat(session.BinPath())
				Expect(errors.Is(sErr, os.ErrNotExist)).To(BeTrue())
			})

			It("tells the client the upload failed", func() {
				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: preparedSession(false).ID(),
					Outcome:  events.PPOutcomeDelete,
				}})

				Expect(pub.published).To(HaveLen(1))
				Expect(pub.published[0].(events.UploadReady).Failed).To(BeTrue())
			})

			It("counts the deletion", func() {
				before := testutil.ToFloat64(metrics.UploadSessionsDeleted)

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: preparedSession(false).ID(),
					Outcome:  events.PPOutcomeDelete,
				}})

				Expect(testutil.ToFloat64(metrics.UploadSessionsDeleted) - before).To(Equal(float64(1)))
			})
		})

		// A newer postprocessing service reporting an outcome this one predates must
		// not leave the node flagged forever.
		Context("an unknown outcome", func() {
			It("is treated as an abort", func() {
				session := preparedSession(true)

				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PostprocessingOutcome("sideways"),
				}})

				Expect(fs.calls).To(Equal([]string{
					"RollbackUpload(nodeExisted=true,sizeDiff=17)",
					"MarkProcessing(false)",
				}))
				_, err := store.Get(ctx, session.ID())
				Expect(err).ToNot(HaveOccurred())
			})

			It("tells the client the upload failed", func() {
				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: preparedSession(false).ID(),
					Outcome:  events.PostprocessingOutcome("sideways"),
				}})

				Expect(pub.published).To(HaveLen(1))
				Expect(pub.published[0].(events.UploadReady).Failed).To(BeTrue())
			})
		})

		// Without the session the staged bytes cannot be reached; housekeeping
		// collects them later.
		It("does nothing for an upload it cannot load", func() {
			c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
				UploadID: "no-such-session",
				Outcome:  events.PPOutcomeContinue,
			}})

			Expect(fs.calls).To(BeEmpty())
			Expect(pub.published).To(BeEmpty())
		})

		// The failure notice names the actor postprocessing reported, not the one
		// who happened to be on the consumer's context.
		It("names the actor postprocessing reported", func() {
			session := preparedSession(false)
			bob := &userpb.User{Id: &userpb.UserId{OpaqueId: "bob"}, Username: "bob"}

			c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
				UploadID:          session.ID(),
				Outcome:           events.PPOutcomeAbort,
				ExecutingUser:     bob,
				ImpersonatingUser: &userpb.User{Id: &userpb.UserId{OpaqueId: "carol"}, Username: "carol"},
			}})

			ready := pub.published[0].(events.UploadReady)
			Expect(ready.ExecutingUser.GetUsername()).To(Equal("bob"))
			Expect(ready.ImpersonatingUser.GetUsername()).To(Equal("carol"))
		})

		It("says nothing when no publisher is wired", func() {
			c = NewCoordinator(fs, store, "", nil)
			session := preparedSession(false)

			c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
				UploadID: session.ID(),
				Outcome:  events.PPOutcomeAbort,
			}})

			Expect(fs.calls).To(ContainElement("MarkProcessing(false)"))
			Expect(pub.published).To(BeEmpty())
		})

		// The upload is already dealt with by then.
		It("does not fail when the notice cannot be published", func() {
			pub.err = errors.New("nats unreachable")

			c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
				UploadID: preparedSession(false).ID(),
				Outcome:  events.PPOutcomeAbort,
			}})

			Expect(fs.calls).To(ContainElement("MarkProcessing(false)"))
		})
	})

	Describe("RestartPostprocessing", func() {
		It("re-publishes BytesReceived so the upload gets another run", func() {
			session := preparedSession(false)

			c.processEvent(ctx, events.Event{Event: events.RestartPostprocessing{UploadID: session.ID()}})

			Expect(pub.published).To(HaveLen(1))
			received := pub.published[0].(events.BytesReceived)
			Expect(received.UploadID).To(Equal(session.ID()))
			Expect(received.Filesize).To(Equal(uint64(bodyLen)))
			Expect(received.URL).To(HavePrefix("https://cloud.example.com/data/"))
		})

		// The node stays flagged and the bytes staged: the restart needs both.
		It("touches neither the node nor the staged bytes", func() {
			session := preparedSession(false)

			c.processEvent(ctx, events.Event{Event: events.RestartPostprocessing{UploadID: session.ID()}})

			Expect(fs.calls).To(BeEmpty())
			_, err := os.Stat(session.BinPath())
			Expect(err).ToNot(HaveOccurred())
		})

		It("counts the restart", func() {
			before := testutil.ToFloat64(metrics.UploadSessionsRestarted)

			c.processEvent(ctx, events.Event{Event: events.RestartPostprocessing{UploadID: preparedSession(false).ID()}})

			Expect(testutil.ToFloat64(metrics.UploadSessionsRestarted) - before).To(Equal(float64(1)))
		})

		It("does nothing for an upload it cannot load", func() {
			c.processEvent(ctx, events.Event{Event: events.RestartPostprocessing{UploadID: "no-such-session"}})

			Expect(pub.published).To(BeEmpty())
		})

		// Postprocessing has no way to reach the bytes without that URL.
		It("survives a download URL it cannot sign", func() {
			session := preparedSession(false)
			c = NewCoordinator(fs, &brokenStore{
				SessionStore: store,
				loadedURLErr: errors.New("could not sign transfer token"),
			}, "", pub)

			c.processEvent(ctx, events.Event{Event: events.RestartPostprocessing{UploadID: session.ID()}})

			Expect(pub.published).To(BeEmpty())
		})
	})

	Describe("CleanUpload", func() {
		It("reverts the node and discards the bytes and the session", func() {
			session := preparedSession(true)

			c.processEvent(ctx, events.Event{Event: events.CleanUpload{UploadID: session.ID()}})

			Expect(fs.calls).To(Equal([]string{
				"RollbackUpload(nodeExisted=true,sizeDiff=17)",
				"MarkProcessing(false)",
			}))
			_, err := store.Get(ctx, session.ID())
			Expect(err).To(HaveOccurred())
			_, sErr := os.Stat(session.BinPath())
			Expect(errors.Is(sErr, os.ErrNotExist)).To(BeTrue())
		})

		It("reverts the node but keeps the bytes and the session when asked to keep the upload", func() {
			session := preparedSession(true)

			c.processEvent(ctx, events.Event{Event: events.CleanUpload{
				UploadID:   session.ID(),
				KeepUpload: true,
			}})

			Expect(fs.calls).To(Equal([]string{
				"RollbackUpload(nodeExisted=true,sizeDiff=17)",
				"MarkProcessing(false)",
			}))
			_, err := store.Get(ctx, session.ID())
			Expect(err).ToNot(HaveOccurred())
			_, sErr := os.Stat(session.BinPath())
			Expect(sErr).ToNot(HaveOccurred())
		})

		// Cleaning up is not something the client waits on.
		It("announces nothing", func() {
			c.processEvent(ctx, events.Event{Event: events.CleanUpload{UploadID: preparedSession(false).ID()}})

			Expect(pub.published).To(BeEmpty())
		})

		It("does nothing for an upload it cannot load", func() {
			c.processEvent(ctx, events.Event{Event: events.CleanUpload{UploadID: "no-such-session"}})

			Expect(fs.calls).To(BeEmpty())
		})
	})

	// Only the antivirus result is recorded on the session, for the commit to carry.
	Describe("PostprocessingStepFinished", func() {
		scanned := func(session Session) (string, time.Time) {
			reloaded, err := store.Get(ctx, session.ID())
			Expect(err).ToNot(HaveOccurred())
			return reloaded.ScanData()
		}

		It("records the scan verdict on the session", func() {
			session := preparedSession(false)

			c.processEvent(ctx, events.Event{Event: events.PostprocessingStepFinished{
				UploadID:     session.ID(),
				FinishedStep: events.PPStepAntivirus,
				Result: events.VirusscanResult{
					Description: "Eicar-Test-Signature",
					Scandate:    time.Unix(1700000000, 0),
				},
			}})

			result, date := scanned(session)
			Expect(result).To(Equal("Eicar-Test-Signature"))
			Expect(date).To(BeTemporally("==", time.Unix(1700000000, 0)))
		})

		It("counts the scan", func() {
			before := testutil.ToFloat64(metrics.UploadSessionsScanned)

			c.processEvent(ctx, events.Event{Event: events.PostprocessingStepFinished{
				UploadID:     preparedSession(false).ID(),
				FinishedStep: events.PPStepAntivirus,
				Result:       events.VirusscanResult{Description: "clean"},
			}})

			Expect(testutil.ToFloat64(metrics.UploadSessionsScanned) - before).To(Equal(float64(1)))
		})

		It("ignores a step that is not the antivirus", func() {
			session := preparedSession(false)

			c.processEvent(ctx, events.Event{Event: events.PostprocessingStepFinished{
				UploadID:     session.ID(),
				FinishedStep: events.PPStepPolicies,
				Result:       events.VirusscanResult{Description: "should not be recorded"},
			}})

			result, _ := scanned(session)
			Expect(result).To(BeEmpty())
		})

		// PostprocessingFinished decides the outcome, so there is no verdict to record.
		It("ignores a scan that failed", func() {
			session := preparedSession(false)

			c.processEvent(ctx, events.Event{Event: events.PostprocessingStepFinished{
				UploadID:     session.ID(),
				FinishedStep: events.PPStepAntivirus,
				Result: events.VirusscanResult{
					Description: "partial",
					ErrorMsg:    "scan engine unavailable",
				},
			}})

			result, _ := scanned(session)
			Expect(result).To(BeEmpty())
		})

		It("ignores a result that is not a scan result", func() {
			session := preparedSession(false)

			c.processEvent(ctx, events.Event{Event: events.PostprocessingStepFinished{
				UploadID:     session.ID(),
				FinishedStep: events.PPStepAntivirus,
				Result:       "not a scan result",
			}})

			result, _ := scanned(session)
			Expect(result).To(BeEmpty())
		})

		It("does nothing for an upload it cannot load", func() {
			c.processEvent(ctx, events.Event{Event: events.PostprocessingStepFinished{
				UploadID:     "no-such-session",
				FinishedStep: events.PPStepAntivirus,
				Result:       events.VirusscanResult{Description: "clean"},
			}})

			Expect(fs.calls).To(BeEmpty())
		})

		// An empty upload id means an on-demand scan, which has no session at all.
		It("accepts an on-demand scan that belongs to no upload", func() {
			c.processEvent(ctx, events.Event{Event: events.PostprocessingStepFinished{
				FinishedStep: events.PPStepAntivirus,
				Result:       events.VirusscanResult{Description: "clean"},
			}})

			Expect(fs.calls).To(BeEmpty())
		})

		// The verdict is only useful if the commit can read it back, so a session that
		// cannot be written keeps none.
		It("survives a session it cannot persist", func() {
			session := preparedSession(false)
			c = NewCoordinator(fs, &brokenStore{SessionStore: store, failPersist: true}, "", pub)

			c.processEvent(ctx, events.Event{Event: events.PostprocessingStepFinished{
				UploadID:     session.ID(),
				FinishedStep: events.PPStepAntivirus,
				Result:       events.VirusscanResult{Description: "clean"},
			}})

			result, _ := scanned(session)
			Expect(result).To(BeEmpty())
		})
	})

	// Every upload that increments the gauge has to decrement it again, on whichever
	// path postprocessing takes.
	Describe("the processing gauge", func() {
		delta := func(body func()) float64 {
			before := testutil.ToFloat64(metrics.UploadProcessing)
			body()
			return testutil.ToFloat64(metrics.UploadProcessing) - before
		}

		DescribeTable("comes back down for an upload that is over",
			func(outcome events.PostprocessingOutcome) {
				session := preparedSession(false)

				Expect(delta(func() {
					c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
						UploadID: session.ID(),
						Outcome:  outcome,
					}})
				})).To(Equal(float64(-1)))
			},
			Entry("on a commit", events.PPOutcomeContinue),
			Entry("on a delete", events.PPOutcomeDelete),
		)

		// The gauge counts processing runs, not uploads, and a restart starts another
		// one. An abort keeps the bytes, so the upload is still in flight and the run
		// it is on has not ended.
		DescribeTable("stays up for an upload that can be restarted",
			func(outcome events.PostprocessingOutcome) {
				session := preparedSession(false)

				Expect(delta(func() {
					c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
						UploadID: session.ID(),
						Outcome:  outcome,
					}})
				})).To(BeZero())
			},
			Entry("on an abort", events.PPOutcomeAbort),
			Entry("on an unknown outcome", events.PostprocessingOutcome("sideways")),
		)

		It("comes back down on a clean-up", func() {
			session := preparedSession(false)

			Expect(delta(func() {
				c.processEvent(ctx, events.Event{Event: events.CleanUpload{UploadID: session.ID()}})
			})).To(Equal(float64(-1)))
		})

		It("comes back down on a clean-up that keeps the bytes", func() {
			session := preparedSession(false)

			Expect(delta(func() {
				c.processEvent(ctx, events.Event{Event: events.CleanUpload{
					UploadID:   session.ID(),
					KeepUpload: true,
				}})
			})).To(Equal(float64(-1)))
		})

		// The upload is still in flight, waiting to be retried.
		It("stays up when the commit fails", func() {
			session := preparedSession(false)
			fs.commitErr = errors.New("blobstore unavailable")

			Expect(delta(func() {
				c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
					UploadID: session.ID(),
					Outcome:  events.PPOutcomeContinue,
				}})
			})).To(BeZero())
		})

		// A restart increments it again, so an aborted upload that is retried has to
		// leave the gauge where the abort left it.
		It("stays down on a restart", func() {
			session := preparedSession(false)

			Expect(delta(func() {
				c.processEvent(ctx, events.Event{Event: events.RestartPostprocessing{UploadID: session.ID()}})
			})).To(BeZero())
		})
	})

	// An async upload end to end: staged and handed off, then cleared and committed.
	Describe("the whole async round trip", func() {
		It("commits the bytes the client uploaded", func() {
			c.async = true
			fs.prepared = &storage.PrepareUploadResult{SizeDiff: bodyLen}
			session := stagedSession(ctx, store, false)

			_, err := c.finishUpload(ctx, session)
			Expect(err).ToNot(HaveOccurred())
			Expect(fs.calls).ToNot(ContainElement(ContainSubstring("CommitUpload")))
			received := pub.published[0].(events.BytesReceived)

			c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
				UploadID: received.UploadID,
				Outcome:  events.PPOutcomeContinue,
			}})

			Expect(fs.calls).To(ContainElement("CommitUpload(length=17)"))
			Expect(fs.committed.Length).To(Equal(bodyLen))
			Expect(pub.published[1].(events.UploadReady).Failed).To(BeFalse())
		})

		// The size PrepareUpload propagated has to be given back, or the quota stays
		// consumed for a file that never landed.
		It("reverts the propagated size when postprocessing rejects it", func() {
			c.async = true
			fs.prepared = &storage.PrepareUploadResult{SizeDiff: bodyLen}
			session := stagedSession(ctx, store, true)

			_, err := c.finishUpload(ctx, session)
			Expect(err).ToNot(HaveOccurred())

			c.processEvent(ctx, events.Event{Event: events.PostprocessingFinished{
				UploadID: session.ID(),
				Outcome:  events.PPOutcomeDelete,
			}})

			Expect(fs.calls).To(ContainElement("RollbackUpload(nodeExisted=true,sizeDiff=17)"))
		})
	})
})
