package eosgrpc

import (
	"context"
	"fmt"
	"io"
	"strconv"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/eosclient"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
	"github.com/cs3org/reva/v3/pkg/trace"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

// Touch creates a 0-size,0-replica file in the EOS namespace.
func (c *Client) Touch(ctx context.Context, auth eosclient.Authorization, path string) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "Touch").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_TouchRequest)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Touch{Touch: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "Touch").Str("path", path).Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' path: '%s'", auth.Role.UID, path))
	}

	log.Debug().Str("func", "Touch").Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err
}

// Chown given path.
func (c *Client) Chown(ctx context.Context, auth, chownAuth eosclient.Authorization, path string) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "Chown").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("chownuid,chowngid", chownAuth.Role.UID+","+chownAuth.Role.GID).Str("path", path).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_ChownRequest)
	msg.Owner = new(erpc.RoleId)

	uid, gid, err := utils.ExtractUidGid(chownAuth)
	if err == nil {
		msg.Owner.Uid = uid
		msg.Owner.Gid = gid
	}

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Chown{Chown: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "Chown").Str("path", path).Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' chownuid: '%s' path: '%s'", auth.Role.UID, chownAuth.Role.UID, path))
	}

	log.Debug().Str("func", "Chown").Str("path", path).Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("chownuid,chowngid", chownAuth.Role.UID+","+chownAuth.Role.GID).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err
}

// Chmod given path.
func (c *Client) Chmod(ctx context.Context, auth eosclient.Authorization, mode, path string) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "Chmod").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("mode", mode).Str("path", path).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_ChmodRequest)

	md, err := strconv.ParseUint(mode, 8, 64)
	if err != nil {
		return err
	}
	msg.Mode = int64(md)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Chmod{Chmod: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "Chmod").Str("path ", path).Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' mode: '%s' path: '%s'", auth.Role.UID, mode, path))
	}

	log.Debug().Str("func", "Chmod").Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err
}

// CreateDir creates a directory at the given path.
func (c *Client) CreateDir(ctx context.Context, auth eosclient.Authorization, path string) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "Createdir").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_MkdirRequest)

	// Let's put 750 as permissions, assuming that EOS will apply some mask
	md, err := strconv.ParseUint("750", 8, 64)
	if err != nil {
		return err
	}
	msg.Mode = int64(md)
	msg.Recursive = true
	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)

	rq.Command = &erpc.NSRequest_Mkdir{Mkdir: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "Createdir").Str("path", path).Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' path: '%s'", auth.Role.UID, path))
	}

	log.Debug().Str("func", "Createdir").Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err
}

func (c *Client) rm(ctx context.Context, auth eosclient.Authorization, path string, noRecycle bool) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "rm").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_UnlinkRequest)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)
	msg.Norecycle = noRecycle

	rq.Command = &erpc.NSRequest_Unlink{Unlink: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "rm").Str("path", path).Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' path: '%s'", auth.Role.UID, path))
	}

	log.Debug().Str("func", "rm").Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err
}

func (c *Client) rmdir(ctx context.Context, auth eosclient.Authorization, path string, noRecycle bool) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "rmdir").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_RmRequest)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(path)
	msg.Recursive = true
	msg.Norecycle = noRecycle

	rq.Command = &erpc.NSRequest_Rm{Rm: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "rmdir").Str("path", path).Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' path: '%s'", auth.Role.UID, path))
	}

	log.Debug().Str("func", "rmdir").Str("path", path).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err
}

// Remove removes the resource at the given path.
func (c *Client) Remove(ctx context.Context, auth eosclient.Authorization, path string, noRecycle bool) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "Remove").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	nfo, err := c.GetFileInfoByPath(ctx, auth, path)
	if err != nil {
		log.Warn().Err(err).Str("func", "Remove").Str("path", path).Str("err", err.Error()).Send()
		return err
	}

	if nfo.IsDir {
		return c.rmdir(ctx, auth, path, noRecycle)
	}

	return c.rm(ctx, auth, path, noRecycle)
}

// Rename renames the resource referenced by oldPath to newPath.
func (c *Client) Rename(ctx context.Context, auth eosclient.Authorization, oldPath, newPath string) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "Rename").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("oldPath", oldPath).Str("newPath", newPath).Msg("")

	// Initialize the common fields of the NSReq
	rq, err := c.initNSRequest(ctx, auth, "")
	if err != nil {
		return err
	}

	msg := new(erpc.NSRequest_RenameRequest)

	msg.Id = new(erpc.MDId)
	msg.Id.Path = []byte(oldPath)
	msg.Target = []byte(newPath)
	rq.Command = &erpc.NSRequest_Rename{Rename: msg}

	// Now send the req and see what happens
	resp, err := c.cl.Exec(appctx.ContextGetClean(ctx), rq)
	e := c.getRespError(resp, err)
	if e != nil {
		log.Error().Str("func", "Rename").Str("oldPath", oldPath).Str("newPath", newPath).Str("err", e.Error()).Msg("")
		return e
	}

	if resp == nil {
		return errtypes.InternalError(fmt.Sprintf("nil response for uid: '%s' oldpath: '%s' newpath: '%s'", auth.Role.UID, oldPath, newPath))
	}

	log.Debug().Str("func", "Rename").Str("oldPath", oldPath).Str("newPath", newPath).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("grpc response")

	return err
}

// List the contents of the directory given by path.
func (c *Client) List(ctx context.Context, auth eosclient.Authorization, path string) ([]*eosclient.FileInfo, error) {
	return c.list(ctx, auth, path, "", 1)
}

func (c *Client) ListWithRegex(ctx context.Context, auth eosclient.Authorization, path string, depth uint, regex string) ([]*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)
	result, err := c.list(ctx, auth, path, regex, depth)
	log.Info().Str("path", path).Str("regex", regex).Any("results", result).Msg("ListWithRegex")

	return result, err
}

// List the contents of the directory given by path.
func (c *Client) list(ctx context.Context, auth eosclient.Authorization, dpath string, regex string, depth uint) ([]*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)

	// Stuff filename, uid, gid into the FindRequest type
	fdrq := new(erpc.FindRequest)
	fdrq.Maxdepth = uint64(depth)
	fdrq.Type = erpc.TYPE_LISTING
	fdrq.Id = new(erpc.MDId)
	fdrq.Id.Path = []byte(dpath)

	fdrq.Role = new(erpc.RoleId)
	fdrq.Role.Trace = trace.Get(ctx)

	uid, gid, err := utils.ExtractUidGid(auth)
	if err == nil {
		fdrq.Role.Uid = uid
		fdrq.Role.Gid = gid

		fdrq.Authkey = c.opt.Authkey
	} else {
		if auth.Token == "" {
			return nil, errors.Wrap(err, "Failed to extract uid/gid from auth")
		}
		fdrq.Authkey = auth.Token
	}

	if regex != "" {
		fdrq.Selection = &erpc.MDSelection{
			RegexpFilename: []byte(regex),
			RegexpDirname:  []byte(regex),
			Select:         true,
		}
	}

	// Now send the req and see what happens
	log.Info().Any("request", fdrq).Msg("dump fdrq")
	resp, err := c.cl.Find(appctx.ContextGetClean(ctx), fdrq)
	log.Info().Str("func", "List").Str("path", dpath).Any("resp", resp).Msg("findrequest grpc response")
	if err != nil {
		log.Error().Err(err).Str("func", "List").Str("path", dpath).Any("resp", resp).Str("err", err.Error()).Msg("findrequest grpc response")

		return nil, err
	}

	var mylst []*eosclient.FileInfo
	versionFolders := map[string]*eosclient.FileInfo{}
	var parent *eosclient.FileInfo
	var ownerAuth *eosclient.Authorization

	i := 0
	for {
		rsp, err := resp.Recv()
		if err != nil {
			if err == io.EOF {
				log.Debug().Str("path", dpath).Int("nitems", i).Msg("OK, no more items, clean exit")
				break
			}

			// We got an error while reading items. We return the error to the user and break off the List operation
			// We do not want to return a partial list, because then a sync client may delete local files that are missing on the server
			log.Error().Err(err).Str("func", "List").Int("nitems", i).Str("path", dpath).Str("got err from EOS", err.Error()).Msg("")
			if i > 0 {
				log.Error().Str("path", dpath).Int("nitems", i).Msg("No more items, dirty exit")
				return nil, errors.Wrap(err, "Error listing files")
			}
		}

		if rsp == nil {
			log.Error().Int("nitems", i).Err(err).Str("func", "List").Str("path", dpath).Str("err", "rsp is nil").Msg("grpc response")
			return nil, errtypes.NotFound(dpath)
		}

		log.Debug().Str("func", "List").Str("path", dpath).Msg("grpc response")

		myitem, err := c.grpcMDResponseToFileInfo(ctx, rsp)
		if err != nil {
			log.Error().Err(err).Str("func", "List").Str("path", dpath).Str("could not convert item:", fmt.Sprintf("%#v", rsp)).Str("err", err.Error()).Msg("")
			return nil, err
		}

		i++
		// The first item is the directory itself... skip
		if i == 1 && regex == "" {
			parent = myitem
			log.Debug().Str("func", "List").Str("path", dpath).Str("skipping first item resp:", fmt.Sprintf("%#v", rsp)).Msg("grpc response")
			continue
		}

		// If it's a version folder, store it in a map, so that for the corresponding file,
		// we can return its inode instead
		if eosclient.IsVersionFolder(myitem.File) {
			versionFolders[myitem.File] = myitem
		} else {
			mylst = append(mylst, myitem)
		}

		if ownerAuth == nil {
			ownerAuth = &eosclient.Authorization{
				Role: eosclient.Role{
					UID: strconv.FormatUint(myitem.UID, 10),
					GID: strconv.FormatUint(myitem.GID, 10),
				},
			}
		}

	}

	log.Info().Any("resp-list", mylst).Msg("FindRequest raw response")

	for _, fi := range mylst {
		if fi.SysACL == nil {
			fi.SysACL = &acl.ACLs{
				Entries: []*acl.Entry{},
			}
		}
		if !fi.IsDir && !eosclient.IsVersionFolder(dpath) {
			// For files, inherit ACLs from the parent
			if parent != nil && parent.SysACL != nil {
				fi.SysACL.Entries = append(fi.SysACL.Entries, parent.SysACL.Entries...)
			}
			// If there is a version folder then use its inode
			// to implement the invariance of the fileid across updates
			versionFolderPath := eosclient.GetVersionFolder(fi.File)
			if vf, ok := versionFolders[versionFolderPath]; ok {
				fi.Inode = vf.Inode
				if vf.SysACL != nil {
					fi.SysACL.Entries = append(fi.SysACL.Entries, vf.SysACL.Entries...)
				}
				for k, v := range vf.Attrs {
					fi.Attrs[k] = v
				}
			} else if err := c.CreateDir(ctx, *ownerAuth, versionFolderPath); err == nil {
				// Create the version folder if it doesn't exist
				if md, err := c.GetFileInfoByPath(ctx, auth, versionFolderPath); err == nil {
					fi.Inode = md.Inode
				} else {
					log.Error().Err(err).Interface("auth", ownerAuth).Str("path", versionFolderPath).Msg("got error creating version folder")
				}
			}
		}
	}

	return mylst, nil
}
