package eosgrpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"path"
	"strconv"
	"strings"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

// GetFileInfoByInode returns the FileInfo by the given inode.
func (c *Client) GetFileInfoByInode(ctx context.Context, auth eosclient.Authorization, inode uint64) (*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("func", "GetFileInfoByInode").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Uint64("inode", inode).Msg("entering")

	// Initialize the common fields of the MDReq
	mdrq, err := c.initMDRequest(ctx, auth)
	if err != nil {
		return nil, err
	}

	// Stuff filename, uid, gid into the MDRequest type
	// TODO this is temporary, until EOS keeps support for both legacy and new inode scheme:
	// we have to do the EOS mapping ourselves and issue a request with the right type.
	// In the future, we should switch back to erpc.TYPE_STAT.
	if inode&(1<<63) != 0 {
		mdrq.Type = erpc.TYPE_FILE
	} else {
		mdrq.Type = erpc.TYPE_CONTAINER
	}
	mdrq.Id = new(erpc.MDId)
	mdrq.Id.Ino = inode

	// Now send the req and see what happens
	resp, err := c.cl.MD(appctx.ContextGetClean(ctx), mdrq)
	if err != nil {
		log.Error().Err(err).Uint64("inode", inode).Str("err", err.Error()).Send()

		return nil, err
	}
	rsp, err := resp.Recv()
	if err != nil {
		log.Error().Err(err).Uint64("inode", inode).Str("err", err.Error()).Send()
		return nil, err
	}

	if rsp == nil {
		return nil, errtypes.InternalError(fmt.Sprintf("nil response for inode: '%d'", inode))
	}

	log.Debug().Uint64("inode", inode).Str("rsp:", fmt.Sprintf("%#v", rsp)).Msg("grpc response")

	info, err := c.grpcMDResponseToFileInfo(ctx, rsp)
	if err != nil {
		return nil, err
	}

	if c.opt.VersionInvariant && eosclient.IsVersionFolder(info.File) {
		info, err = c.getFileInfoFromVersion(ctx, auth, info.File)
		if err != nil {
			return nil, err
		}
		info.Inode = inode
	}

	log.Info().Str("func", "GetFileInfoByInode").Uint64("inode", inode).Uint64("info.Inode", info.Inode).Str("file", info.File).Uint64("size", info.Size).Str("etag", info.ETag).Msg("result")
	return c.fixupACLs(ctx, auth, info), nil
}

// GetFileInfoByPath returns the FilInfo at the given path.
func (c *Client) GetFileInfoByPath(ctx context.Context, userAuth eosclient.Authorization, path string) (*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("func", "GetFileInfoByPath").Str("uid,gid", userAuth.Role.UID+","+userAuth.Role.GID).Str("path", path).Msg("entering")

	// UserAuth may not be sufficient, because the user may not have access to the file
	// e.g. in the case of a guest account. So we check if a uid/gid is set, and if not,
	// revert to the daemon account
	auth := utils.GetUserOrDaemonAuth(userAuth)

	// Initialize the common fields of the MDReq
	mdrq, err := c.initMDRequest(ctx, auth)
	if err != nil {
		return nil, err
	}

	mdrq.Type = erpc.TYPE_STAT
	mdrq.Id = new(erpc.MDId)
	mdrq.Id.Path = []byte(path)

	// Now send the req and see what happens
	resp, err := c.cl.MD(appctx.ContextGetClean(ctx), mdrq)
	if err != nil {
		log.Error().Str("func", "GetFileInfoByPath").Err(err).Str("path", path).Str("err", err.Error()).Msg("")

		return nil, err
	}
	rsp, err := resp.Recv()
	if err != nil {
		log.Error().Str("func", "GetFileInfoByPath").Err(err).Str("path", path).Str("err", err.Error()).Msg("")

		// FIXME: this is very bad and poisonous for the project!!!!!!!
		// Apparently here we have to assume that an error in Recv() means "file not found"
		// - "File not found is not an error", it's a legitimate result of a legitimate check
		// - Assuming that any error means file not found is doubly poisonous
		return nil, errtypes.NotFound(err.Error())
		// return nil, nil
	}

	if rsp == nil {
		return nil, errtypes.NotFound(fmt.Sprintf("%s:%s", "acltype", path))
	}

	log.Debug().Str("func", "GetFileInfoByPath").Str("path", path).Str("rsp:", fmt.Sprintf("%#v", rsp)).Msg("grpc response")

	info, err := c.grpcMDResponseToFileInfo(ctx, rsp)
	if err != nil {
		return nil, err
	}

	if c.opt.VersionInvariant && !eosclient.IsVersionFolder(path) && !info.IsDir {
		// Here we have to create a missing version folder, irrespective from the user (that could be a sharee, or a lw account, or...)
		// Therefore, we impersonate the owner of the file
		ownerAuth := eosclient.Authorization{
			Role: eosclient.Role{
				UID: strconv.FormatUint(info.UID, 10),
				GID: strconv.FormatUint(info.GID, 10),
			},
		}

		inode, err := c.getOrCreateVersionFolderInode(ctx, ownerAuth, path)
		if err != nil {
			return nil, err
		}
		info.Inode = inode
	}

	log.Info().Str("func", "GetFileInfoByPath").Str("path", path).Uint64("info.Inode", info.Inode).Uint64("size", info.Size).Str("etag", info.ETag).Msg("result")
	return c.fixupACLs(ctx, auth, info), nil
}

// GetFileInfoByFXID returns the FileInfo by the given file id in hexadecimal.
func (c *Client) GetFileInfoByFXID(ctx context.Context, auth eosclient.Authorization, fxid string) (*eosclient.FileInfo, error) {
	return nil, errtypes.NotSupported("eosgrpc: GetFileInfoByFXID not implemented")
}

func (c *Client) getFileInfoFromVersion(ctx context.Context, auth eosclient.Authorization, p string) (*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "getFileInfoFromVersion").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("p", p).Msg("")

	file := eosclient.GetFileFromVersionFolder(p)
	md, err := c.GetFileInfoByPath(ctx, auth, file)
	if err != nil {
		return nil, err
	}
	return md, nil
}

func (c *Client) grpcMDResponseToFileInfo(ctx context.Context, st *erpc.MDResponse) (*eosclient.FileInfo, error) {
	if st.Cmd == nil && st.Fmd == nil {
		return nil, errors.Wrap(errtypes.NotSupported(""), "Invalid response (st.Cmd and st.Fmd are nil)")
	}
	fi := new(eosclient.FileInfo)

	log := appctx.GetLogger(ctx)

	if st.Type == erpc.TYPE_CONTAINER {
		fi.IsDir = true
		fi.Inode = st.Cmd.Inode
		fi.FID = st.Cmd.ParentId
		fi.UID = st.Cmd.Uid
		fi.GID = st.Cmd.Gid
		fi.MTimeSec = st.Cmd.Mtime.Sec
		// For directories, we prefer stime over mtime
		if st.Cmd.Stime != nil {
			fi.MTimeSec = st.Cmd.Stime.Sec
		}
		fi.ETag = st.Cmd.Etag
		fi.File = path.Clean(string(st.Cmd.Path))

		fi.Attrs = make(map[string]string)
		for k, v := range st.Cmd.Xattrs {
			fi.Attrs[strings.TrimPrefix(k, "user.")] = string(v)
		}

		if fi.Attrs["sys.acl"] != "" {
			fi.SysACL = aclAttrToAclStruct(fi.Attrs["sys.acl"])
		}

		fi.TreeSize = uint64(st.Cmd.TreeSize)
		fi.Size = fi.TreeSize
		fi.TreeCount = st.Cmd.Files + st.Cmd.Containers

		log.Debug().Str("stat file path", fi.File).Uint64("inode", fi.Inode).Uint64("uid", fi.UID).Uint64("gid", fi.GID).Str("etag", fi.ETag).Msg("grpc response")
	} else {
		fi.Inode = st.Fmd.Inode
		fi.FID = st.Fmd.ContId
		fi.UID = st.Fmd.Uid
		fi.GID = st.Fmd.Gid
		fi.MTimeSec = st.Fmd.Mtime.Sec
		fi.ETag = st.Fmd.Etag
		fi.File = path.Clean(string(st.Fmd.Path))

		fi.Attrs = make(map[string]string)
		for k, v := range st.Fmd.Xattrs {
			fi.Attrs[strings.TrimPrefix(k, "user.")] = string(v)
		}

		if fi.Attrs["sys.acl"] != "" {
			fi.SysACL = aclAttrToAclStruct(fi.Attrs["sys.acl"])
		}

		fi.Size = st.Fmd.Size

		if st.Fmd.Checksum != nil {
			xs := &eosclient.Checksum{
				XSSum:  hex.EncodeToString(st.Fmd.Checksum.Value),
				XSType: st.Fmd.Checksum.Type,
			}
			fi.XS = xs

			log.Debug().Str("stat folder path", fi.File).Uint64("inode", fi.Inode).Uint64("uid", fi.UID).Uint64("gid", fi.GID).Str("etag", fi.ETag).Str("checksum", fi.XS.XSType+":"+fi.XS.XSSum).Msg("grpc response")
		}
	}
	return fi, nil
}
