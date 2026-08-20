package reconciliation

import (
	"path"
	"slices"

	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage/fs/eos/acl"
)

// TODO(jgeens):
// - there is no need to keep the "root ACLs" in every node,
//   we can just append them at the end
// - for "wide" trees, we could sort the children and do a binary search
//   over them instead of iterating over all children
// - we should guard `Insert` agains concurrent Inserts

// An ACL Tree represents the tree of ACLs in a space.
// ACLTree's do not *require* in-order insertion, but note that pre-sorting the paths
// that will be inserted (so that parents are always inserted before their children)
// significantly improves its performance.
//
// e.g.:
// BenchmarkInsert/1365_nodes/top_down         10117 µs   1370 KB    16204 allocs
// BenchmarkInsert/1365_nodes/deepest_first   505674 µs  84681 KB  1427558 allocs
type ACLTree struct {
	SpaceType spaces.SpaceType
	ACLNode
}

type ACLNode struct {
	Path          string
	MandatoryACLs []*acl.Entry
	AllowedACLs   []*acl.Entry
	Children      []*ACLNode
}

func NewACLTree() *ACLTree {
	return &ACLTree{}
}

func (n *ACLNode) Insert(i *ACLNode) bool {
	i.Path = path.Clean(i.Path)
	return n.insert(i)
}

// The internal-only insert does not clean i's path, so we only need to do this once
// and not again for all its children
func (n *ACLNode) insert(i *ACLNode) bool {
	// Node n cannot be a parent of node i
	if !n.Matches(i.Path) {
		return false
	}

	// First, check if any of the children match
	for _, c := range n.Children {
		if ok := c.insert(i); ok {
			return true
		}
	}

	// Node i matches under Node n but not any of its children.
	// That means:
	// - if it is under a different path, it becomes a new child
	// - if it has the same path, we need to merge the two

	if n.MatchesExact(i.Path) {
		// Two nodes are for the same path, so we need to merge
		n.ApplyACLs(i.MandatoryACLs, i.AllowedACLs)
		n.Children = append(n.Children, i.Children...)
	} else {
		// Child
		// In this case, we should still merge! ACLs are set recursively
		// So, we apply n's ACLs to i and then insert it
		i.ApplyACLs(n.MandatoryACLs, n.AllowedACLs)

		// Nodes are not guaranteed to be inserted in order, so we may need to insert
		// i into any of the children of n instead of in n itself.
		var stay, move []*ACLNode
		for _, c := range n.Children {
			if i.Matches(c.Path) {
				// Child, should be removed from n and added to i
				// i will run the same check again for its children
				move = append(move, c)
			} else {
				stay = append(stay, c)
			}
		}
		// We need to insert - not append - children, becaus
		for _, c := range move {
			i.insert(c)
		}
		n.Children = append(stay, i)

	}
	return true
}

// Return true if this node or any of its children
// match the given path
func (n *ACLNode) Matches(p string) bool {
	if p == n.Path {
		return true
	}
	if p == "/" {
		return true
	}
	return fastHasPrefix(p, n.Path)
}

// faster implementation of the previous check:
//
//	`_, ok := strings.CutPrefix(p, n.Path+"/")`
func fastHasPrefix(s, prefix string) bool {
	return len(s) > len(prefix) && s[len(prefix)] == '/' && s[:len(prefix)] == prefix
}

// Return true if the node's path is `p`
func (n *ACLNode) MatchesExact(p string) bool {
	return p == n.Path
}

func (n *ACLNode) Find(p string) (MandatoryACLs, AllowedACLs []*acl.Entry, ok bool) {
	return n.find(path.Clean(p))
}

// The internal-only find does not clean the path p, so we only need to do this once
// and not for every visited node
func (n *ACLNode) find(p string) (MandatoryACLs, AllowedACLs []*acl.Entry, ok bool) {
	if !n.Matches(p) {
		return nil, nil, false
	}

	for _, c := range n.Children {
		m, a, ok := c.find(p)
		if ok {
			return m, a, true
		}
	}

	return n.MandatoryACLs, n.AllowedACLs, true
}

func (n *ACLNode) ApplyACLs(mandatory, optional []*acl.Entry) {
	n.MandatoryACLs = mergeACLs(n.MandatoryACLs, mandatory)
	n.AllowedACLs = mergeACLs(n.AllowedACLs, optional)
	for _, c := range n.Children {
		c.ApplyACLs(mandatory, optional)
	}
}

func mergeACLs(a, b []*acl.Entry) []*acl.Entry {
	resultSet := slices.Clone(a)
	for _, e := range b {

		// Now we need to check: is there already
		// an entry in a for this qualifier + type?
		foundMatch := false
		for i, c := range resultSet {
			if e.Qualifier == c.Qualifier && e.Type == c.Type {
				// ACL for the same user: highest permission wins

				// We don't want to override other references to this ACL, so we make a copy
				merged := *c
				merged.Permissions = highestPermission(e.Permissions, c.Permissions)
				resultSet[i] = &merged

				foundMatch = true
				break
			}
		}

		// Otherwise, no ACL for this user in the resultSet yet,
		// so we add it
		if !foundMatch {
			resultSet = append(resultSet, e)
		}
	}

	return resultSet
}

type Permission int

const (
	permissionNone Permission = iota

	permissionRead
	permissionWrite
	permissionDeny
)

var aclPermissions = map[string]Permission{
	"rx":     permissionRead,
	"rwx":    permissionWrite,
	"!r!w!x": permissionDeny, // TODO: verify exact ACL for deny
}

func highestPermission(a, b string) string {
	pa, pb := aclPermissions[a], aclPermissions[b]
	switch {
	case pa == permissionNone && pb == permissionNone:
		return a
	case pa >= pb:
		return a
	default:
		return b
	}
}
