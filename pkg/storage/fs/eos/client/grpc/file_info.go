package eosgrpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"syscall"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// mdRecvError classifies the error a streaming MD Recv returned. An MDResponse
// carries no error of its own, so the outcome of a lookup is only readable from
// the stream, and EOS reports a missing entry in two different ways depending on
// what was asked of it (GrpcNsInterface::GetMD):
//
//   - a file looked up by id ends the stream without sending anything, which
//     surfaces here as io.EOF;
//   - a container looked up by id, and anything looked up by path with
//     TYPE_STAT, comes back as a status error instead.
//
// Those status errors do not always carry a grpc code: EOS casts the errno of
// the namespace exception straight into the code, so a missing entry arrives as
// ENOENT, the number grpc reads back as UNKNOWN. Both that and a real
// codes.NotFound are taken as an absence. Every other value is a real failure -
// an unreachable MGM, a timeout, a rejected caller - and is returned unchanged
// so it stays distinguishable. The callers above depend on the difference: the
// storage provider turns a not-found into CODE_NOT_FOUND and anything else into
// CODE_INTERNAL, which is what tells a caller "this resource no longer exists"
// apart from "we could not find out".
func mdRecvError(err error, target string) error {
	if errors.Is(err, io.EOF) {
		return errtypes.NotFound(target)
	}
	if st, ok := grpcstatus.FromError(err); ok {
		switch st.Code() {
		case codes.NotFound, codes.Code(syscall.ENOENT):
			return errtypes.NotFound(target)
		}
	}
	return err
}

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
		return nil, mdRecvError(err, fmt.Sprintf("inode: '%d'", inode))
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
	return c.fixupACLsAndAttrs(ctx, auth, info), nil
}

// GetFileInfoByPath returns the FilInfo at the given path.
func (c *Client) GetFileInfoByPath(ctx context.Context, auth eosclient.Authorization, path string) (*eosclient.FileInfo, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("func", "GetFileInfoByPath").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("entering")

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

		return nil, mdRecvError(err, fmt.Sprintf("path: %s", path))
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
	return c.fixupACLsAndAttrs(ctx, auth, info), nil
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
		fi.Mode = uint64(st.Cmd.Mode)
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
		// EOS stores the file mode bits in the FileMd flags field.
		fi.Mode = uint64(st.Fmd.Flags)
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
