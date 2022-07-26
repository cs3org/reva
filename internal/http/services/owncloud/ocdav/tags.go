package ocdav

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/rhttp/router"
	"github.com/cs3org/reva/v2/pkg/storagespace"
)

// character to seperate tags
var _tagsep = ","

// TagHandler handles meta requests
type TagHandler struct {
}

func (h *TagHandler) init(c *Config) error {
	return nil
}

// Handler handles requests
func (h *TagHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		id, _ := router.ShiftPath(r.URL.Path)
		did, err := storagespace.ParseID(id)
		if err != nil {
			logger := appctx.GetLogger(r.Context())
			logger.Debug().Str("prop", net.PropOcMetaPathForUser).Msg("invalid resource id")
			w.WriteHeader(http.StatusBadRequest)
			m := fmt.Sprintf("Invalid resource id %v", id)
			b, err := errors.Marshal(http.StatusBadRequest, m, "")
			errors.HandleWebdavError(logger, w, b, err)
			return
		}

		switch r.Method {
		default:
			w.WriteHeader(http.StatusBadRequest)
		case http.MethodPut:
			h.handleCreateTags(w, r, s, &did)
		case http.MethodDelete:
			h.handleDeleteTags(w, r, s, &did)
		}

	})
}

func (h *TagHandler) handleCreateTags(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Interface("resourceid", rid).Logger()

	newtags := strings.ToLower(r.FormValue("tags"))
	if newtags == "" {
		w.WriteHeader(http.StatusBadRequest)
		b, err := errors.Marshal(http.StatusNotFound, "no tags in createtagsrequest", "")
		errors.HandleWebdavError(&log, w, b, err)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	oldtags, err := getExistingTags(ctx, client, rid)
	if err != nil {
		log.Error().Err(err).Msg("error getting existing tags")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	tags := FromString(oldtags)
	ok := tags.AddString(newtags)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		b, err := errors.Marshal(http.StatusNotFound, "no new tags in createtagsrequest", "")
		errors.HandleWebdavError(&log, w, b, err)
		return
	}

	_, err = client.SetArbitraryMetadata(ctx, &provider.SetArbitraryMetadataRequest{
		Ref: &provider.Reference{ResourceId: rid},
		ArbitraryMetadata: &provider.ArbitraryMetadata{
			Metadata: map[string]string{
				"tags": tags.AsString(),
			},
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("error setting tags")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *TagHandler) handleDeleteTags(w http.ResponseWriter, r *http.Request, s *svc, rid *provider.ResourceId) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Interface("resourceid", rid).Logger()

	todelete := strings.ToLower(r.FormValue("tags"))
	if todelete == "" {
		w.WriteHeader(http.StatusBadRequest)
		b, err := errors.Marshal(http.StatusNotFound, "no tags in createtagsrequest", "")
		errors.HandleWebdavError(&log, w, b, err)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	oldtags, err := getExistingTags(ctx, client, rid)
	if err != nil {
		log.Error().Err(err).Msg("error stating item")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	tags := FromString(oldtags)
	tags.RemoveString(todelete)
	if _, err = client.SetArbitraryMetadata(ctx, &provider.SetArbitraryMetadataRequest{
		Ref: &provider.Reference{ResourceId: rid},
		ArbitraryMetadata: &provider.ArbitraryMetadata{
			Metadata: map[string]string{
				"tags": tags.AsString(),
			},
		},
	}); err != nil {
		log.Error().Err(err).Msg("error setting tags")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getExistingTags(ctx context.Context, client gateway.GatewayAPIClient, rid *provider.ResourceId) (string, error) {
	// check for existing tags - could also be done by the storage provider, but then it has to know about tags!?
	sres, err := client.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{ResourceId: rid},
	})
	if err != nil {
		return "", err
	}

	if m := sres.GetInfo().GetArbitraryMetadata().GetMetadata(); m != nil {
		return m["tags"], nil
	}

	return "", nil
}

// Tags is a helper struct for merging, deleting and deduplicating the tags while preserving the order
type Tags struct {
	t      []string
	sep    string
	exists map[string]bool
}

// FromString creates a Tags struct from a string
func FromString(s string) *Tags {
	t := &Tags{sep: _tagsep, exists: make(map[string]bool)}

	tags := strings.Split(s, t.sep)
	for _, tag := range tags {
		t.t = append(t.t, tag)
		t.exists[tag] = true
	}
	return t
}

// AddString appends the the new tags and returns true if at least one was appended
func (t *Tags) AddString(s string) bool {
	var tags []string
	for _, tag := range strings.Split(s, t.sep) {
		if !t.exists[tag] {
			tags = append(tags, tag)
			t.exists[tag] = true
		}
	}

	t.t = append(tags, t.t...)
	return len(tags) != 0
}

// RemoveString removes the the tags
func (t *Tags) RemoveString(s string) {
	for _, tag := range strings.Split(s, t.sep) {
		if !t.exists[tag] {
			// should this be reported?
			continue
		}

		for i, tt := range t.t {
			if tt == tag {
				t.t = append(t.t[:i], t.t[i+1:]...)
				break
			}
		}

		delete(t.exists, tag)
	}
}

// AsString returns the tags converted to a string
func (t *Tags) AsString() string {
	return strings.Join(t.t, t.sep)
}
