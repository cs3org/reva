package ocdavsvc

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

type chunkHeaderInfo struct {
	// OC-Chunked = 1
	ochunked bool

	// OC-Chunk-Size
	ocChunkSize uint64

	// OC-Total-Length
	ocTotalLength uint64
}

type chunkBLOBInfo struct {
	path         string
	transferID   string
	totalChunks  int64
	currentChunk int64
}

// not using the resource path in the chunk folder name allows uploading
// to the same folder after a move without having to restart the chunk
// upload
func (c *chunkBLOBInfo) uploadID() string {
	return fmt.Sprintf("chunking-%s-%d", c.transferID, c.totalChunks)
}

func getChunkBLOBInfo(path string) (*chunkBLOBInfo, error) {
	parts := strings.Split(path, "-chunking-")
	tail := strings.Split(parts[1], "-")

	totalChunks, err := strconv.ParseInt(tail[1], 10, 64)
	if err != nil {
		return nil, err
	}

	currentChunk, err := strconv.ParseInt(tail[2], 10, 64)
	if err != nil {
		return nil, err
	}
	if currentChunk >= totalChunks {
		return nil, fmt.Errorf("current chunk:%d exceeds total number of chunks:%d", currentChunk, totalChunks)
	}

	return &chunkBLOBInfo{
		path:         parts[0],
		transferID:   tail[0],
		totalChunks:  totalChunks,
		currentChunk: currentChunk,
	}, nil
}

func (s *svc) createChunkTempFile() (string, *os.File, error) {
	file, err := ioutil.TempFile(fmt.Sprintf("/%s", s.chunkFolder), "")
	if err != nil {
		return "", nil, err
	}

	return file.Name(), file, nil
}

func (s *svc) getChunkFolderName(i *chunkBLOBInfo) (string, error) {
	path := "/" + s.chunkFolder + filepath.Clean("/"+i.uploadID())
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}
	return path, nil
}

func (s *svc) saveChunk(ctx context.Context, path string, r io.ReadCloser) (bool, string, error) {
	chunkInfo, err := getChunkBLOBInfo(path)
	if err != nil {
		err := fmt.Errorf("error getting chunk info from path: %s", path)
		//c.logger.Error().Log("error", err)
		return false, "", err
	}

	//c.logger.Info().Log("chunknum", chunkInfo.currentChunk, "chunks", chunkInfo.totalChunks,
	//"transferid", chunkInfo.transferID, "uploadid", chunkInfo.uploadID())

	chunkTempFilename, chunkTempFile, err := s.createChunkTempFile()
	if err != nil {
		//c.logger.Error().Log("error", err)
		return false, "", err
	}
	defer chunkTempFile.Close()

	if _, err := io.Copy(chunkTempFile, r); err != nil {
		//c.logger.Error().Log("error", err)
		return false, "", err
	}

	// force close of the file here because if it is the last chunk to
	// assemble the big file we must have all the chunks already closed.
	if err = chunkTempFile.Close(); err != nil {
		//c.logger.Error().Log("error", err)
		return false, "", err
	}

	chunksFolderName, err := s.getChunkFolderName(chunkInfo)
	if err != nil {
		//c.logger.Error().Log("error", err)
		return false, "", err
	}
	//c.logger.Info().Log("chunkfolder", chunksFolderName)

	chunkTarget := chunksFolderName + "/" + fmt.Sprintf("%d", chunkInfo.currentChunk)
	if err = os.Rename(chunkTempFilename, chunkTarget); err != nil {
		//c.logger.Error().Log("error", err)
		return false, "", err
	}

	//c.logger.Info().Log("chunktarget", chunkTarget)

	// Check that all chunks are uploaded.
	// This is very inefficient, the server has to check that it has all the
	// chunks after each uploaded chunk.
	// A two-phase upload like DropBox is better, because the server will
	// assembly the chunks when the client asks for it.
	chunksFolder, err := os.Open(chunksFolderName)
	if err != nil {
		//c.logger.Error().Log("error", err)
		return false, "", err
	}
	defer chunksFolder.Close()

	// read all the chunks inside the chunk folder; -1 == all
	chunks, err := chunksFolder.Readdir(-1)
	if err != nil {
		//c.logger.Error().Log("error", err)
		return false, "", err
	}
	//c.logger.Info().Log("msg", "chunkfolder readed", "nchunks", len(chunks))

	// there is still some chunks to be uploaded.
	// we return CodeUploadIsPartial to notify uper layers that the upload is still
	// not complete and requires more actions.
	// This code is needed to notify the owncloud webservice that the upload has not yet been
	// completed and needs to continue uploading chunks.
	if len(chunks) < int(chunkInfo.totalChunks) {
		return false, "", nil
	}

	assembledFileName, assembledFile, err := s.createChunkTempFile()
	if err != nil {
		//c.logger.Error().Log("error", err)
		return false, "", err
	}
	defer assembledFile.Close()

	//c.logger.Info().Log("assembledfile", assembledFileName)

	// walk all chunks and append to assembled file
	for i := range chunks {
		target := chunksFolderName + "/" + fmt.Sprintf("%d", i)

		chunk, err := os.Open(target)
		if err != nil {
			//c.logger.Error().Log("error", err)
			return false, "", err
		}
		defer chunk.Close()

		if _, err = io.Copy(assembledFile, chunk); err != nil {
			//c.logger.Error().Log("error", err)
			return false, "", err
		}
		//c.logger.Debug().Log("msg", "chunk appended to assembledfile")

		// we close the chunk here because if the assembled file contains hundreds of chunks
		// we will end up with hundreds of open file descriptors
		if err = chunk.Close(); err != nil {
			//c.logger.Error().Log("error", err)
			return false, "", err

		}
	}

	// at this point the assembled file is complete
	// so we free space removing the chunks folder
	defer func() {
		if err = os.RemoveAll(chunksFolderName); err != nil {
			//c.logger.Crit().Log("error", err, "msg", "error deleting chunk folder")
		}
	}()

	// when writing to the assembled file the write pointer points to the end of the file
	// so we need to seek it to the beginning
	if _, err = assembledFile.Seek(0, 0); err != nil {
		//c.logger.Error().Log("error", err)
		return false, "", err
	}

	tempFileName := assembledFileName
	return true, tempFileName, nil
}
func (s *svc) doPutChunked(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fn := r.URL.Path

	if r.Body == nil {
		logger.Println(ctx, "body is nil")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	finish, chunk, err := s.saveChunk(ctx, fn, r.Body)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !finish {
		w.WriteHeader(http.StatusPartialContent)
		return
	}

	fd, err := os.Open(chunk)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer fd.Close()

	chunkInfo, _ := getChunkBLOBInfo(fn)

	client, err := s.getClient()
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &storageproviderv0alphapb.StatRequest{Filename: chunkInfo.path}
	res, err := client.Stat(ctx, req)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		if res.Status.Code != rpcpb.Code_CODE_NOT_FOUND {
			logger.Println(ctx, res.Status)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

	}

	md := res.Metadata
	if md != nil && md.IsDir {
		logger.Println(ctx, "resource is a folder")
		w.WriteHeader(http.StatusConflict)
		return
	}

	if md != nil {
		clientETag := r.Header.Get("If-Match")
		serverETag := md.Etag
		if clientETag != "" {
			serverETag = fmt.Sprintf(`"%s"`, serverETag)
			if clientETag != serverETag {
				logger.Build().Str("client-etag", clientETag).Str("server-etag", serverETag).Msg(ctx, "etags mismatch")
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		}
	}

	req2 := &storageproviderv0alphapb.StartWriteSessionRequest{}
	res2, err := client.StartWriteSession(ctx, req2)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res2.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, res2.Status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sessID := res2.SessionId
	logger.Build().Str("sessID", sessID).Msg(ctx, "got write session id")

	stream, err := client.Write(ctx)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	buffer := make([]byte, 1024*1024*3)
	var offset uint64
	var numChunks uint64

	for {
		n, err := fd.Read(buffer)
		if n > 0 {
			req := &storageproviderv0alphapb.WriteRequest{Data: buffer, Length: uint64(n), SessionId: sessID, Offset: offset}
			err = stream.Send(req)
			if err != nil {
				logger.Error(ctx, err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			numChunks++
			offset += uint64(n)
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			logger.Error(ctx, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	res3, err := stream.CloseAndRecv()
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res3.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req4 := &storageproviderv0alphapb.FinishWriteSessionRequest{Filename: chunkInfo.path, SessionId: sessID}
	res4, err := client.FinishWriteSession(ctx, req4)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res4.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, res4.Status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req.Filename = chunkInfo.path
	res, err = client.Stat(ctx, req)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		logger.Println(ctx, res.Status)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	md2 := res.Metadata

	w.Header().Add("Content-Type", md2.Mime)
	w.Header().Set("ETag", md2.Etag)
	w.Header().Set("OC-FileId", md2.Id)
	w.Header().Set("OC-ETag", md2.Etag)
	t := time.Unix(int64(md2.Mtime), 0)
	lastModifiedString := t.Format(time.RFC1123)
	w.Header().Set("Last-Modified", lastModifiedString)
	w.Header().Set("X-OC-MTime", "accepted")

	if md == nil {
		w.WriteHeader(http.StatusCreated)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	return
}
