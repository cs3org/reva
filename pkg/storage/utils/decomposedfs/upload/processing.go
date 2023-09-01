// Copyright 2018-2022 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package upload

import (
	"context"
	"path/filepath"
	"time"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/events"
	"github.com/cs3org/reva/v2/pkg/logger"
	"github.com/cs3org/reva/v2/pkg/rhttp/datatx/manager/tus"
	"github.com/cs3org/reva/v2/pkg/storage/cache"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/tree"
	"github.com/cs3org/reva/v2/pkg/utils"
	tusd "github.com/tus/tusd/pkg/handler"
)

// PermissionsChecker defines an interface for checking permissions on a Node
type PermissionsChecker interface {
	AssemblePermissions(ctx context.Context, n *node.Node) (ap provider.ResourcePermissions, err error)
}

type Propagator interface {
	Propagate(ctx context.Context, node *node.Node, sizeDiff int64) (err error)
}

// Postprocessing starts the postprocessing result collector
func Postprocessing(lu *lookup.Lookup, propagator Propagator, cache cache.StatCache, es events.Stream, tusDataStore tusd.DataStore, blobstore tree.Blobstore, downloadURLfunc func(ctx context.Context, id string) (string, error), ch <-chan events.Event) {
	ctx := context.TODO() // we should pass the trace id in the event and initialize the trace provider here
	ctx, span := tracer.Start(ctx, "Postprocessing")
	defer span.End()
	log := logger.New()
	for event := range ch {
		switch ev := event.Event.(type) {
		case events.PostprocessingFinished:
			up, err := tusDataStore.GetUpload(ctx, ev.UploadID)
			if err != nil {
				log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("Failed to get upload")
				continue // NOTE: since we can't get the upload, we can't delete the blob
			}
			info, err := up.GetInfo(ctx)
			if err != nil {
				log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("Failed to get upload info")
				continue // NOTE: since we can't get the upload, we can't delete the blob
			}

			var (
				failed     bool
				keepUpload bool
			)

			var sizeDiff int64
			// propagate sizeDiff after failed postprocessing

			n, err := ReadNode(ctx, lu, info)
			if err != nil {
				log.Error().Err(err).Str("uploadID", ev.UploadID).
					Str("space", info.MetaData[tus.CS3Prefix+"SpaceRoot"]).
					Str("parent", info.MetaData[tus.CS3Prefix+"NodeParentId"]).
					Str("node", info.MetaData[tus.CS3Prefix+"NodeId"]).
					Str("revision", info.MetaData[tus.CS3Prefix+"RevisionTime"]).
					Str("name", info.MetaData[tus.CS3Prefix+"filename"]).
					Msg("could not read revision node")
				continue
			}

			switch ev.Outcome {
			default:
				log.Error().Str("outcome", string(ev.Outcome)).Str("uploadID", ev.UploadID).Msg("unknown postprocessing outcome - aborting")
				fallthrough
			case events.PPOutcomeAbort:
				failed = true
				keepUpload = true
			case events.PPOutcomeContinue:
				if err := Finalize(ctx, blobstore, info, n); err != nil {
					log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("could not finalize upload")
					keepUpload = true // should we keep the upload when assembling failed?
					failed = true
				}
				sizeDiff, err = SetNodeToRevision(ctx, lu, n, info.MetaData[tus.CS3Prefix+"RevisionTime"])
				if err != nil {
					log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("could set node to revision upload")
					keepUpload = true // should we keep the upload when assembling failed?
					failed = true
				}
			case events.PPOutcomeDelete:
				failed = true
			}

			getParent := func() *node.Node {
				p, err := n.Parent(ctx)
				if err != nil {
					log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("could not read parent")
					return nil
				}
				return p
			}

			now := time.Now()
			if failed {
				// propagate sizeDiff after failed postprocessing
				if err := propagator.Propagate(ctx, n, -sizeDiff); err != nil { // FIXME revert sizediff .,.. and write an issue that condemns this
					log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("could not propagate tree size change")
				}

			} else if p := getParent(); p != nil {
				// update parent tmtime to propagate etag change after successful postprocessing
				_ = p.SetTMTime(ctx, &now)
				if err := propagator.Propagate(ctx, p, 0); err != nil {
					log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("could not propagate etag change")
				}
			}

			Cleanup(ctx, lu, n, info, failed)
			if !keepUpload {
				if tup, ok := up.(tusd.TerminatableUpload); ok {
					terr := tup.Terminate(ctx)
					if terr != nil {
						log.Error().Err(terr).Msg("failed to terminate upload")
					}
				}
			}

			// remove cache entry in gateway
			cache.RemoveStatContext(ctx, ev.ExecutingUser.GetId(), &provider.ResourceId{SpaceId: n.SpaceID, OpaqueId: n.ID})

			if err := events.Publish(
				ctx,
				es,
				events.UploadReady{
					UploadID: ev.UploadID,
					Failed:   failed,
					ExecutingUser: &user.User{
						Id: &user.UserId{
							Type:     user.UserType(user.UserType_value[info.MetaData[tus.CS3Prefix+"ExecutantType"]]),
							Idp:      info.MetaData[tus.CS3Prefix+"ExecutantIdp"],
							OpaqueId: info.MetaData[tus.CS3Prefix+"ExecutantId"],
						},
						Username: info.MetaData[tus.CS3Prefix+"ExecutantUserName"],
					},
					Filename: ev.Filename,
					FileRef: &provider.Reference{
						ResourceId: &provider.ResourceId{
							StorageId: info.MetaData[tus.CS3Prefix+"providerID"],
							SpaceId:   info.MetaData[tus.CS3Prefix+"SpaceRoot"],
							OpaqueId:  info.MetaData[tus.CS3Prefix+"SpaceRoot"],
						},
						// FIXME this seems wrong, path is not really relative to space root
						// actually it is: InitiateUpload calls fs.lu.Path to get the path relative to the root so soarch can index the path
						// hm is that robust? what if the file is moved? shouldn't we store the parent id, then?
						Path: utils.MakeRelativePath(filepath.Join(info.MetaData[tus.CS3Prefix+"dir"], info.MetaData[tus.CS3Prefix+"filename"])),
					},
					Timestamp:  utils.TimeToTS(now),
					SpaceOwner: n.SpaceOwnerOrManager(ctx),
				},
			); err != nil {
				log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("Failed to publish UploadReady event")
			}
		case events.RestartPostprocessing:
			up, err := tusDataStore.GetUpload(ctx, ev.UploadID)
			if err != nil {
				log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("Failed to get upload")
				continue // NOTE: since we can't get the upload, we can't restart postprocessing
			}
			info, err := up.GetInfo(ctx)
			if err != nil {
				log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("Failed to get upload info")
				continue // NOTE: since we can't get the upload, we can't restart postprocessing
			}

			n, err := ReadNode(ctx, lu, info)
			if err != nil {
				log.Error().Err(err).Str("uploadID", ev.UploadID).
					Str("space", info.MetaData[tus.CS3Prefix+"SpaceRoot"]).
					Str("parent", info.MetaData[tus.CS3Prefix+"NodeParentId"]).
					Str("node", info.MetaData[tus.CS3Prefix+"NodeId"]).
					Str("revision", info.MetaData[tus.CS3Prefix+"RevisionTime"]).
					Str("name", info.MetaData[tus.CS3Prefix+"filename"]).
					Msg("could not read revision node")
				continue
			}

			s, err := downloadURLfunc(ctx, ev.UploadID)
			if err != nil {
				log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("could not create url")
				continue
			}
			// restart postprocessing
			if err := events.Publish(ctx, es, events.BytesReceived{
				UploadID:      info.ID,
				URL:           s,
				SpaceOwner:    n.SpaceOwnerOrManager(ctx),
				ExecutingUser: &user.User{Id: &user.UserId{OpaqueId: "postprocessing-restart"}}, // send nil instead?
				ResourceID:    &provider.ResourceId{SpaceId: n.SpaceID, OpaqueId: n.ID},
				Filename:      info.MetaData[tus.CS3Prefix+"filename"],
				Filesize:      uint64(info.Size),
			}); err != nil {
				log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("Failed to publish BytesReceived event")
			}
		case events.PostprocessingStepFinished:
			if ev.FinishedStep != events.PPStepAntivirus {
				// atm we are only interested in antivirus results
				continue
			}

			res := ev.Result.(events.VirusscanResult)
			if res.ErrorMsg != "" {
				// scan failed somehow
				// Should we handle this here?
				continue
			}

			var n *node.Node
			switch ev.UploadID {
			case "":
				// uploadid is empty -> this was an on-demand scan
				/* ON DEMAND SCANNING NOT SUPPORTED ATM
				ctx := ctxpkg.ContextSetUser(context.Background(), ev.ExecutingUser)
				ref := &provider.Reference{ResourceId: ev.ResourceID}

				no, err := fs.lu.NodeFromResource(ctx, ref)
				if err != nil {
					log.Error().Err(err).Interface("resourceID", ev.ResourceID).Msg("Failed to get node after scan")
					continue

				}
				n = no
				if ev.Outcome == events.PPOutcomeDelete {
					// antivir wants us to delete the file. We must obey and need to

					// check if there a previous versions existing
					revs, err := fs.ListRevisions(ctx, ref)
					if len(revs) == 0 {
						if err != nil {
							log.Error().Err(err).Interface("resourceID", ev.ResourceID).Msg("Failed to list revisions. Fallback to delete file")
						}

						// no versions -> trash file
						err := fs.Delete(ctx, ref)
						if err != nil {
							log.Error().Err(err).Interface("resourceID", ev.ResourceID).Msg("Failed to delete infected resource")
							continue
						}

						// now purge it from the recycle bin
						if err := fs.PurgeRecycleItem(ctx, &provider.Reference{ResourceId: &provider.ResourceId{SpaceId: n.SpaceID, OpaqueId: n.SpaceID}}, n.ID, "/"); err != nil {
							log.Error().Err(err).Interface("resourceID", ev.ResourceID).Msg("Failed to purge infected resource from trash")
						}

						// remove cache entry in gateway
						fs.cache.RemoveStatContext(ctx, ev.ExecutingUser.GetId(), &provider.ResourceId{SpaceId: n.SpaceID, OpaqueId: n.ID})
						continue
					}

					// we have versions - find the newest
					versions := make(map[uint64]string) // remember all versions - we need them later
					var nv uint64
					for _, v := range revs {
						versions[v.Mtime] = v.Key
						if v.Mtime > nv {
							nv = v.Mtime
						}
					}

					// restore newest version
					if err := fs.RestoreRevision(ctx, ref, versions[nv]); err != nil {
						log.Error().Err(err).Interface("resourceID", ev.ResourceID).Str("revision", versions[nv]).Msg("Failed to restore revision")
						continue
					}

					// now find infected version
					revs, err = fs.ListRevisions(ctx, ref)
					if err != nil {
						log.Error().Err(err).Interface("resourceID", ev.ResourceID).Msg("Error listing revisions after restore")
					}

					for _, v := range revs {
						// we looking for a version that was previously not there
						if _, ok := versions[v.Mtime]; ok {
							continue
						}

						if err := fs.DeleteRevision(ctx, ref, v.Key); err != nil {
							log.Error().Err(err).Interface("resourceID", ev.ResourceID).Str("revision", v.Key).Msg("Failed to delete revision")
						}
					}

					// remove cache entry in gateway
					fs.cache.RemoveStatContext(ctx, ev.ExecutingUser.GetId(), &provider.ResourceId{SpaceId: n.SpaceID, OpaqueId: n.ID})
					continue
				}
				*/
			default:
				// uploadid is not empty -> this is an async upload
				up, err := tusDataStore.GetUpload(ctx, ev.UploadID)
				if err != nil {
					log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("Failed to get upload")
					continue
				}
				info, err := up.GetInfo(ctx)
				if err != nil {
					log.Error().Err(err).Str("uploadID", ev.UploadID).Msg("Failed to get upload info")
					continue
				}

				// scan data should be set on the node revision not the node ... then when postprocessing finishes we need to copy the state to the node

				n, err = ReadNode(ctx, lu, info)
				if err != nil {
					log.Error().Err(err).Str("uploadID", ev.UploadID).
						Str("space", info.MetaData[tus.CS3Prefix+"SpaceRoot"]).
						Str("parent", info.MetaData[tus.CS3Prefix+"NodeParentId"]).
						Str("node", info.MetaData[tus.CS3Prefix+"NodeId"]).
						Str("revision", info.MetaData[tus.CS3Prefix+"RevisionTime"]).
						Str("name", info.MetaData[tus.CS3Prefix+"filename"]).
						Msg("could not read revision node")
					continue
				}
			}

			if err := n.SetScanData(ctx, res.Description, res.Scandate); err != nil {
				log.Error().Err(err).Str("uploadID", ev.UploadID).Interface("resourceID", res.ResourceID).Msg("Failed to set scan results")
				continue
			}

			// remove cache entry in gateway
			cache.RemoveStatContext(ctx, ev.ExecutingUser.GetId(), &provider.ResourceId{SpaceId: n.SpaceID, OpaqueId: n.ID})
		default:
			log.Error().Interface("event", ev).Msg("Unknown event")
		}
	}
}
