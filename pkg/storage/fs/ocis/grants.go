package ocis

import (
	"context"
	"encoding/csv"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/pkg/xattr"
)

func (fs *ocisfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Interface("ref", ref).Interface("grant", g).Msg("AddGrant()")
	var node *NodeInfo
	if node, err = fs.pw.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}

	e, err := fs.getACE(g)
	if err != nil {
		return err
	}

	var attr string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		attr = sharePrefix + "g:" + e.Principal
	} else {
		attr = sharePrefix + "u:" + e.Principal
	}

	np := filepath.Join(fs.pw.Root(), "nodes", node.ID)
	if err := xattr.Set(np, attr, getValue(e)); err != nil {
		return err
	}
	return fs.tp.Propagate(ctx, node)
}

func getValue(e *ace) []byte {
	// first byte will be replaced after converting to byte array
	val := fmt.Sprintf("_t=%s:f=%s:p=%s", e.Type, e.Flags, e.Permissions)
	b := []byte(val)
	b[0] = 0 // indicate key value
	return b
}

func getACEPerm(set *provider.ResourcePermissions) (string, error) {
	var b strings.Builder

	if set.Stat || set.InitiateFileDownload || set.ListContainer {
		b.WriteString("r")
	}
	if set.InitiateFileUpload || set.Move {
		b.WriteString("w")
	}
	if set.CreateContainer {
		b.WriteString("a")
	}
	if set.Delete {
		b.WriteString("d")
	}

	// sharing
	if set.AddGrant || set.RemoveGrant || set.UpdateGrant {
		b.WriteString("C")
	}
	if set.ListGrants {
		b.WriteString("c")
	}

	// trash
	if set.ListRecycle {
		b.WriteString("u")
	}
	if set.RestoreRecycleItem {
		b.WriteString("U")
	}
	if set.PurgeRecycle {
		b.WriteString("P")
	}

	// versions
	if set.ListFileVersions {
		b.WriteString("v")
	}
	if set.RestoreFileVersion {
		b.WriteString("V")
	}

	// quota
	if set.GetQuota {
		b.WriteString("q")
	}
	// TODO set quota permission?
	// TODO GetPath
	return b.String(), nil
}

func (fs *ocisfs) getACE(g *provider.Grant) (*ace, error) {
	permissions, err := getACEPerm(g.Permissions)
	if err != nil {
		return nil, err
	}
	e := &ace{
		Principal:   g.Grantee.Id.OpaqueId,
		Permissions: permissions,
		// TODO creator ...
		Type: "A",
	}
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		e.Flags = "g"
	}
	return e, nil
}

type ace struct {
	//NFSv4 acls
	Type        string // t
	Flags       string // f
	Principal   string // im key
	Permissions string // p

	// sharing specific
	ShareTime int    // s
	Creator   string // c
	Expires   int    // e
	Password  string // w passWord TODO h = hash
	Label     string // l
}

func unmarshalACE(v []byte) (*ace, error) {
	// first byte indicates type of value
	switch v[0] {
	case 0: // = ':' separated key=value pairs
		s := string(v[1:])
		return unmarshalKV(s)
	default:
		return nil, fmt.Errorf("unknown ace encoding")
	}
}

func unmarshalKV(s string) (*ace, error) {
	e := &ace{}
	r := csv.NewReader(strings.NewReader(s))
	r.Comma = ':'
	r.Comment = 0
	r.FieldsPerRecord = -1
	r.LazyQuotes = false
	r.TrimLeadingSpace = false
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) != 1 {
		return nil, fmt.Errorf("more than one row of ace kvs")
	}
	for i := range records[0] {
		kv := strings.Split(records[0][i], "=")
		switch kv[0] {
		case "t":
			e.Type = kv[1]
		case "f":
			e.Flags = kv[1]
		case "p":
			e.Permissions = kv[1]
		case "s":
			v, err := strconv.Atoi(kv[1])
			if err != nil {
				return nil, err
			}
			e.ShareTime = v
		case "c":
			e.Creator = kv[1]
		case "e":
			v, err := strconv.Atoi(kv[1])
			if err != nil {
				return nil, err
			}
			e.Expires = v
		case "w":
			e.Password = kv[1]
		case "l":
			e.Label = kv[1]
			// TODO default ... log unknown keys? or add as opaque? hm we need that for tagged shares ...
		}
	}
	return e, nil
}

// Parse parses an acl string with the given delimiter (LongTextForm or ShortTextForm)
func getACEs(ctx context.Context, fsfn string, attrs []string) (entries []*ace, err error) {
	log := appctx.GetLogger(ctx)
	entries = []*ace{}
	for i := range attrs {
		if strings.HasPrefix(attrs[i], sharePrefix) {
			principal := attrs[i][len(sharePrefix):]
			var value []byte
			if value, err = xattr.Get(fsfn, attrs[i]); err != nil {
				log.Error().Err(err).Str("attr", attrs[i]).Msg("could not read attribute")
				continue
			}
			var e *ace
			if e, err = unmarshalACE(value); err != nil {
				log.Error().Err(err).Str("attr", attrs[i]).Msg("could unmarshal ace")
				continue
			}
			e.Principal = principal[2:]
			// check consistency of Flags and principal type
			if strings.Contains(e.Flags, "g") {
				if principal[:1] != "g" {
					log.Error().Str("attr", attrs[i]).Interface("ace", e).Msg("inconsistent ace: expected group")
					continue
				}
			} else {
				if principal[:1] != "u" {
					log.Error().Str("attr", attrs[i]).Interface("ace", e).Msg("inconsistent ace: expected user")
					continue
				}
			}
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func (fs *ocisfs) ListGrants(ctx context.Context, ref *provider.Reference) (grants []*provider.Grant, err error) {
	var node *NodeInfo
	if node, err = fs.pw.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}
	log := appctx.GetLogger(ctx)
	np := filepath.Join(fs.pw.Root(), "nodes", node.ID)
	var attrs []string
	if attrs, err = xattr.List(np); err != nil {
		log.Error().Err(err).Msg("error listing attributes")
		return nil, err
	}

	log.Debug().Interface("attrs", attrs).Msg("read attributes")
	// filter attributes
	var aces []*ace
	if aces, err = getACEs(ctx, np, attrs); err != nil {
		log.Error().Err(err).Msg("error getting aces")
		return nil, err
	}

	grants = make([]*provider.Grant, 0, len(aces))
	for i := range aces {
		grantee := &provider.Grantee{
			// TODO lookup uid from principal
			Id:   &userpb.UserId{OpaqueId: aces[i].Principal},
			Type: fs.getGranteeType(aces[i]),
		}
		grants = append(grants, &provider.Grant{
			Grantee:     grantee,
			Permissions: fs.getGrantPermissionSet(aces[i].Permissions),
		})
	}

	return grants, nil
}

func (fs *ocisfs) getGranteeType(e *ace) provider.GranteeType {
	if strings.Contains(e.Flags, "g") {
		return provider.GranteeType_GRANTEE_TYPE_GROUP
	}
	return provider.GranteeType_GRANTEE_TYPE_USER
}

func (fs *ocisfs) getGrantPermissionSet(mode string) *provider.ResourcePermissions {
	p := &provider.ResourcePermissions{}
	// r
	if strings.Contains(mode, "r") {
		p.Stat = true
		p.InitiateFileDownload = true
		p.ListContainer = true
	}
	// w
	if strings.Contains(mode, "w") {
		p.InitiateFileUpload = true
		if p.InitiateFileDownload {
			p.Move = true
		}
	}
	//a
	if strings.Contains(mode, "a") {
		// TODO append data to file permission?
		p.CreateContainer = true
	}
	//x
	//if strings.Contains(mode, "x") {
	// TODO execute file permission?
	// TODO change directory permission?
	//}
	//d
	if strings.Contains(mode, "d") {
		p.Delete = true
	}
	//D ?

	// sharing
	if strings.Contains(mode, "C") {
		p.AddGrant = true
		p.RemoveGrant = true
		p.UpdateGrant = true
	}
	if strings.Contains(mode, "c") {
		p.ListGrants = true
	}

	// trash
	if strings.Contains(mode, "u") { // u = undelete
		p.ListRecycle = true
	}
	if strings.Contains(mode, "U") {
		p.RestoreRecycleItem = true
	}
	if strings.Contains(mode, "P") {
		p.PurgeRecycle = true
	}

	// versions
	if strings.Contains(mode, "v") {
		p.ListFileVersions = true
	}
	if strings.Contains(mode, "V") {
		p.RestoreFileVersion = true
	}

	// ?
	// TODO GetPath
	if strings.Contains(mode, "q") {
		p.GetQuota = true
	}
	// TODO set quota permission?
	return p
}

func (fs *ocisfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	var node *NodeInfo
	if node, err = fs.pw.NodeFromResource(ctx, ref); err != nil {
		return
	}
	if !node.Exists {
		err = errtypes.NotFound(filepath.Join(node.ParentID, node.Name))
		return
	}

	var attr string
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		attr = sharePrefix + "g:" + g.Grantee.Id.OpaqueId
	} else {
		attr = sharePrefix + "u:" + g.Grantee.Id.OpaqueId
	}

	np := filepath.Join(fs.pw.Root(), "nodes", node.ID)
	if err = xattr.Remove(np, attr); err != nil {
		return
	}

	return fs.tp.Propagate(ctx, node)
}

func (fs *ocisfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return fs.AddGrant(ctx, ref, g)
}
