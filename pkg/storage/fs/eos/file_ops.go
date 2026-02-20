// Copyright 2018-2026 CERN
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

package eos

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/storage"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/v3/pkg/storage/utils/templates"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

func (fs *Eosfs) CreateDir(ctx context.Context, ref *provider.Reference) error {
	log := appctx.GetLogger(ctx)

	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}

	// TODO: do we need the auth corresponding to the parent directory?
	// so should do path.Dir(p) ?
	//fn := fs.wrap(ctx, p)
	auth, err := fs.getUserAuthOrToken(ctx, fn)
	if err != nil {
		return err
	}

	log.Info().Msgf("eosfs: createdir: path=%s", fn)
	return fs.c.CreateDir(ctx, auth, fn)
}

// TODO: uses systemAuth here: we should do an auth check for the user!
func (fs *Eosfs) CreateReference(ctx context.Context, fn string, targetURI *url.URL) error {
	_, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: no user in ctx")
	}

	// TODO(labkode): with the grpc plugin we can create a file touching with xattrs.
	// Current mechanism is: touch to hidden location, set xattr, rename.
	if !fs.isPathWrapped(fn) {
		fn = fs.wrap(ctx, fn)
	}
	dir, base := path.Split(fn)
	tmp := path.Join(dir, fmt.Sprintf(".sys.reva#.%s", base))
	sysAuth := getSystemAuth()

	if err := fs.c.CreateDir(ctx, sysAuth, tmp); err != nil {
		err = errors.Wrapf(err, "eosfs: error creating temporary ref file")
		return err
	}

	// set xattr on ref
	attr := &eosclient.Attribute{
		Type: UserAttr,
		Key:  refTargetAttrKey,
		Val:  targetURI.String(),
	}

	if err := fs.c.SetAttr(ctx, sysAuth, attr, false, false, tmp, ""); err != nil {
		err = errors.Wrapf(err, "eosfs: error setting reva.ref attr on file: %q", tmp)
		return err
	}

	// rename to have the file visible in user space.
	if err := fs.c.Rename(ctx, sysAuth, tmp, fn); err != nil {
		err = errors.Wrapf(err, "eosfs: error renaming from: %q to %q", tmp, fn)
		return err
	}

	return nil
}

// Create a new, empty file
func (fs *Eosfs) TouchFile(ctx context.Context, ref *provider.Reference) error {
	log := appctx.GetLogger(ctx)
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return err
	}

	auth, err := fs.getUserAuthOrToken(ctx, fn)
	if err != nil {
		return err
	}

	log.Info().Msgf("eosfs: touch file: path=%s", fn)

	return fs.c.Touch(ctx, auth, fn)
}

// TODO: is token on oldPath enough?
func (fs *Eosfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	oldPath, err := fs.resolve(ctx, oldRef)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}
	newPath, err := fs.resolve(ctx, newRef)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}

	// oldFn := fs.wrap(ctx, oldPath)
	// newFn := fs.wrap(ctx, newPath)

	auth, err := fs.getUserAuthOrToken(ctx, oldPath)
	if err != nil {
		return err
	}

	return fs.c.Rename(ctx, auth, oldPath, newPath)
}

func (fs *Eosfs) Delete(ctx context.Context, ref *provider.Reference) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return err
	}
	//fn = fs.wrap(ctx, fn)

	auth, err := fs.getUserAuthOrToken(ctx, fn)
	if err != nil {
		return err
	}

	return fs.c.Remove(ctx, auth, fn, false)
}

func (fs *Eosfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, metadata map[string]string) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "eos: error resolving reference")
	}

	ok, err := chunking.IsChunked(fn)
	if err != nil {
		return errors.Wrap(err, "eos: error checking path")
	}
	if ok {
		var assembledFile string
		fn, assembledFile, err = fs.chunkHandler.WriteChunk(fn, r)
		if err != nil {
			return err
		}
		if fn == "" {
			return errtypes.PartialContent(ref.String())
		}
		fd, err := os.Open(assembledFile)
		if err != nil {
			return errors.Wrap(err, "eos: error opening assembled file")
		}
		defer fd.Close()
		defer os.RemoveAll(assembledFile)
		r = fd
	}

	//fn := fs.wrap(ctx, p)

	// We need the auth corresponding to the parent directory
	// as the file might not exist at the moment
	auth, err := fs.getUserAuthOrToken(ctx, path.Dir(fn))
	if err != nil {
		return err
	}

	if metadata == nil {
		metadata = map[string]string{}
	}
	app := metadata["lockholder"]
	// if we have a lock context, the app for EOS must match the lock holder, else we just tag the traffic as write
	if app == "" {
		app = "write"
	}
	disableVersioning, err := strconv.ParseBool(metadata["disableVersioning"])
	if err != nil {
		disableVersioning = false
	}
	contentLength := metadata[ocdav.HeaderContentLength]
	if contentLength == "" {
		contentLength = metadata[ocdav.HeaderUploadLength]
	}
	len, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return errtypes.BadRequest("no content length specified in EOS upload")
	}

	return fs.c.Write(ctx, auth, fn, r, len, fs.EncodeAppName(app), disableVersioning)
}

func (fs *Eosfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	p, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"simple": p,
	}, nil
}

func (fs *Eosfs) CreateHome(ctx context.Context) error {
	if !fs.conf.EnableHomeCreation {
		return errtypes.NotSupported("eosfs: create home not supported")
	}

	if err := fs.createNominalHome(ctx); err != nil {
		return errors.Wrap(err, "eosfs: error creating nominal home")
	}

	return nil
}

func (fs *Eosfs) Download(ctx context.Context, ref *provider.Reference, ranges []storage.Range) (io.ReadCloser, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, err
	}

	auth, err := fs.getUserAuthOrToken(ctx, fn)
	if err != nil {
		return nil, err
	}

	return fs.c.Read(ctx, auth, fn, ranges)
}

func (fs *Eosfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	p, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error resolving reference")
	}

	return fs.listWithNominalHome(ctx, p)
}

func (fs *Eosfs) ListWithRegex(ctx context.Context, path, regex string, depth uint, user *userpb.User) ([]*provider.ResourceInfo, error) {
	userAuth, err := fs.getUserAuthOrToken(ctx, path)
	if err != nil {
		return nil, err
	}

	eosFileInfos, err := fs.c.ListWithRegex(ctx, userAuth, path, depth, regex)
	if err != nil {
		return nil, err
	}
	resourceInfos := []*provider.ResourceInfo{}

	for _, eosFileInfo := range eosFileInfos {
		// filter out sys folders

		finfo, err := fs.convertToResourceInfo(ctx, eosFileInfo)
		if err == nil && !eosclient.IsVersionFolder(finfo.Path) {
			resourceInfos = append(resourceInfos, finfo)
		}
	}

	return resourceInfos, err
}

func (fs *Eosfs) listWithNominalHome(ctx context.Context, p string) (finfos []*provider.ResourceInfo, err error) {
	log := appctx.GetLogger(ctx)
	fn := fs.wrap(ctx, p)

	userAuth, err := fs.getUserAuthOrToken(ctx, fn)
	if err != nil {
		return nil, err
	}

	eosFileInfos, err := fs.c.List(ctx, userAuth, fn)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "PermissionDenied"):
			return nil, errtypes.PermissionDenied(err.Error())
		case strings.Contains(err.Error(), "NotFound"):
			return nil, errtypes.NotFound(err.Error())
		default:
			return nil, errors.Wrap(err, "eosfs: error listing")
		}
	}

	for _, eosFileInfo := range eosFileInfos {
		// filter out sys files
		if !fs.conf.ShowHiddenSysFiles {
			base := path.Base(eosFileInfo.File)
			if hiddenReg.MatchString(base) {
				log.Debug().Msgf("eosfs: path is filtered because is considered hidden: path=%s hiddenReg=%s", base, hiddenReg)
				continue
			}
		}

		// Remove the hidden folders in the topmost directory
		if finfo, err := fs.convertToResourceInfo(ctx, eosFileInfo); err == nil &&
			finfo.Path != "/" && !strings.HasPrefix(finfo.Path, "/.") {
			finfos = append(finfos, finfo)
		}
	}

	return finfos, nil
}

func (fs *Eosfs) createNominalHome(ctx context.Context) error {
	log := appctx.GetLogger(ctx)

	u, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: no user in ctx")
	}

	if utils.IsLightweightUser(u) {
		return fmt.Errorf("eosfs: lightweight users cannot create homes")
	}

	home := templates.WithUser(u, fs.conf.UserLayout)
	home = path.Join(fs.conf.Namespace, home)

	auth, err := fs.getUserAuth(ctx)
	if err != nil {
		return err
	}

	_, err = fs.c.GetFileInfoByPath(ctx, auth, home)
	if err == nil { // home already exists
		log.Error().Str("home", home).Msg("Home already exists")
		return nil
	}

	if _, ok := err.(errtypes.IsNotFound); !ok {
		return errors.Wrap(err, "eosfs: error verifying if user home directory exists")
	}

	log.Info().Interface("user", u.Id).Interface("home", home).Msg("creating user home")

	if fs.conf.CreateHomeHook != "" {
		hook := exec.Command(fs.conf.CreateHomeHook, u.Username, utils.UserTypeToString(u.Id.Type))
		err = hook.Run()
		log.Info().Interface("output", hook.Stdout).Err(err).Msg("create_home_hook output")
		if err != nil {
			return errors.Wrap(err, "eosfs: error running create home hook")
		}
	} else {
		log.Fatal().Msg("create_home_hook not configured")
		return errtypes.NotFound("eosfs: create home hook not configured")
	}

	return nil
}
