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

package reconciliation

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
)

// sameEntries reports whether got and want hold the same ACL entries. The tree
// gives no order guarantee, so both sides are sorted before the comparison.
func sameEntries(got, want []acl.Entry) bool {
	cmp := func(a, b acl.Entry) int {
		if c := strings.Compare(a.Type, b.Type); c != 0 {
			return c
		}
		if c := strings.Compare(a.Qualifier, b.Qualifier); c != 0 {
			return c
		}
		return strings.Compare(a.Permissions, b.Permissions)
	}
	g, w := slices.Clone(got), slices.Clone(want)
	slices.SortFunc(g, cmp)
	slices.SortFunc(w, cmp)
	return slices.Equal(g, w)
}

// checkFind looks up p and compares both ACL sets of the node that applies.
func checkFind(t *testing.T, tree *ACLTree, p string, wantMandatory, wantAllowed []acl.Entry) {
	t.Helper()
	mandatory, allowed, ok := tree.Find(p)
	if !ok {
		t.Fatalf("Find(%q): ok = false, want true", p)
	}
	if !sameEntries(mandatory, wantMandatory) {
		t.Errorf("Find(%q): mandatory = %v, want %v", p, mandatory, wantMandatory)
	}
	if !sameEntries(allowed, wantAllowed) {
		t.Errorf("Find(%q): allowed = %v, want %v", p, allowed, wantAllowed)
	}
}

var (
	aliceRead  = acl.Entry{Type: acl.TypeUser, Qualifier: "alice", Permissions: "rx"}
	aliceWrite = acl.Entry{Type: acl.TypeUser, Qualifier: "alice", Permissions: "rwx"}
	bobRead    = acl.Entry{Type: acl.TypeUser, Qualifier: "bob", Permissions: "rx"}
	groupRead  = acl.Entry{Type: acl.TypeGroup, Qualifier: "cernbox-admins", Permissions: "rx"}
	external   = acl.Entry{Type: acl.TypeUser, Qualifier: "cboxexternal", Permissions: "rwx"}
)

// An empty tree has no rule, so every path is found with no ACL at all.
func TestFindOnEmptyTree(t *testing.T) {
	tree := NewACLTree()
	checkFind(t, tree, "/eos/project/c/cernbox/some/file", nil, nil)
}

// A rule applies to its own path and to everything below it, and to nothing
// outside of it.
func TestRuleAppliesRecursively(t *testing.T) {
	tree := NewACLTree()
	tree.Insert(&ACLNode{
		Path:          "/eos/project/c/cernbox/shared",
		MandatoryACLs: []acl.Entry{aliceRead},
	})

	checkFind(t, tree, "/eos/project/c/cernbox/shared", []acl.Entry{aliceRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/shared/sub/deep", []acl.Entry{aliceRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/other", nil, nil)
	// a sibling with a common prefix but a different path segment
	checkFind(t, tree, "/eos/project/c/cernbox/sharedother", nil, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/shared-doc.md", nil, nil)
}

// A deeper rule adds its entity to the ones inherited from above.
func TestDeeperRuleAddsEntity(t *testing.T) {
	tree := NewACLTree()
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox", MandatoryACLs: []acl.Entry{aliceRead}})
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox/sub", MandatoryACLs: []acl.Entry{bobRead}})

	checkFind(t, tree, "/eos/project/c/cernbox", []acl.Entry{aliceRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/sub", []acl.Entry{aliceRead, bobRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/sub/deep", []acl.Entry{aliceRead, bobRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/elsewhere", []acl.Entry{aliceRead}, nil)
}

// A deeper rule raises the permission of an entity, and never lowers it.
func TestDeeperRuleRaisesPermission(t *testing.T) {
	t.Run("raise", func(t *testing.T) {
		tree := NewACLTree()
		tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox", MandatoryACLs: []acl.Entry{aliceRead}})
		tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox/sub", MandatoryACLs: []acl.Entry{aliceWrite}})

		checkFind(t, tree, "/eos/project/c/cernbox", []acl.Entry{aliceRead}, nil)
		checkFind(t, tree, "/eos/project/c/cernbox/sub", []acl.Entry{aliceWrite}, nil)
	})

	t.Run("no lowering", func(t *testing.T) {
		tree := NewACLTree()
		tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox", MandatoryACLs: []acl.Entry{aliceWrite}})
		tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox/sub", MandatoryACLs: []acl.Entry{aliceRead}})

		checkFind(t, tree, "/eos/project/c/cernbox/sub", []acl.Entry{aliceWrite}, nil)
	})
}

// Rules do not arrive in tree order, so the result must not depend on the
// insertion order. Insert takes the node over, so every order gets its own
// nodes.
func TestInsertOrderDoesNotMatter(t *testing.T) {
	newNodes := func() []*ACLNode {
		return []*ACLNode{
			{Path: "/eos/project/c/cernbox", MandatoryACLs: []acl.Entry{aliceRead}},
			{Path: "/eos/project/c/cernbox/a", MandatoryACLs: []acl.Entry{bobRead}},
			{Path: "/eos/project/c/cernbox/a/b", MandatoryACLs: []acl.Entry{groupRead}},
		}
	}

	orders := [][]int{
		{0, 1, 2},
		{0, 2, 1},
		{1, 0, 2},
		{1, 2, 0},
		{2, 0, 1},
		{2, 1, 0},
	}

	for _, order := range orders {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			nodes := newNodes()
			tree := NewACLTree()
			for _, i := range order {
				tree.Insert(nodes[i])
			}

			checkFind(t, tree, "/eos/project/c/cernbox", []acl.Entry{aliceRead}, nil)
			checkFind(t, tree, "/eos/project/c/cernbox/a", []acl.Entry{aliceRead, bobRead}, nil)
			checkFind(t, tree, "/eos/project/c/cernbox/a/b", []acl.Entry{aliceRead, bobRead, groupRead}, nil)
			checkFind(t, tree, "/eos/project/c/cernbox/a/b/c", []acl.Entry{aliceRead, bobRead, groupRead}, nil)
		})
	}
}

// A path can carry more than one rule. The rules merge into one node, so an
// entity appears one time only.
func TestTwoRulesOnTheSamePath(t *testing.T) {
	tree := NewACLTree()
	p := "/eos/project/c/cernbox/shared"
	tree.Insert(&ACLNode{Path: p, MandatoryACLs: []acl.Entry{aliceRead}})
	tree.Insert(&ACLNode{Path: p, MandatoryACLs: []acl.Entry{bobRead}})
	tree.Insert(&ACLNode{Path: p, MandatoryACLs: []acl.Entry{aliceWrite}})

	checkFind(t, tree, p, []acl.Entry{aliceWrite, bobRead}, nil)
	checkFind(t, tree, p+"/sub", []acl.Entry{aliceWrite, bobRead}, nil)
}

// A second rule on a path that already has a subtree must reach that subtree
// too.
func TestSecondRuleOnAPathWithChildren(t *testing.T) {
	tree := NewACLTree()
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox", MandatoryACLs: []acl.Entry{aliceRead}})
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox/sub", MandatoryACLs: []acl.Entry{bobRead}})
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox", MandatoryACLs: []acl.Entry{groupRead}})

	checkFind(t, tree, "/eos/project/c/cernbox", []acl.Entry{aliceRead, groupRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/sub", []acl.Entry{aliceRead, bobRead, groupRead}, nil)
}

// A rule can sit below a path that has no rule of its own.
func TestGapBetweenRules(t *testing.T) {
	tree := NewACLTree()
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox", MandatoryACLs: []acl.Entry{aliceRead}})
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox/a/b/c", MandatoryACLs: []acl.Entry{bobRead}})

	checkFind(t, tree, "/eos/project/c/cernbox/a", []acl.Entry{aliceRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/a/b", []acl.Entry{aliceRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/a/b/c", []acl.Entry{aliceRead, bobRead}, nil)
}

// Find cleans the path it gets, so a caller can pass a path as it comes from
// the namespace.
func TestFindCleansThePath(t *testing.T) {
	tree := NewACLTree()
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox", MandatoryACLs: []acl.Entry{aliceRead}})

	checkFind(t, tree, "/eos/project/c/cernbox/", []acl.Entry{aliceRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/sub/..", []acl.Entry{aliceRead}, nil)
	checkFind(t, tree, "/eos/project/c/other/../cernbox/sub", []acl.Entry{aliceRead}, nil)
}

// Sibling subtrees stay independent.
func TestSiblingsAreIndependent(t *testing.T) {
	tree := NewACLTree()
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox/a", MandatoryACLs: []acl.Entry{aliceRead}})
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox/b", MandatoryACLs: []acl.Entry{bobRead}})

	checkFind(t, tree, "/eos/project/c/cernbox/a/deep", []acl.Entry{aliceRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/b/deep", []acl.Entry{bobRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox", nil, nil)
}

// Mandatory and allowed ACLs are two independent sets on the same node.
func TestMandatoryAndAllowedAreSeparate(t *testing.T) {
	tree := NewACLTree()
	tree.Insert(&ACLNode{
		Path:          "/eos/project/c/cernbox",
		MandatoryACLs: []acl.Entry{aliceRead},
		AllowedACLs:   []acl.Entry{external},
	})
	tree.Insert(&ACLNode{
		Path:          "/eos/project/c/cernbox/sub",
		MandatoryACLs: []acl.Entry{bobRead},
	})

	checkFind(t, tree, "/eos/project/c/cernbox", []acl.Entry{aliceRead}, []acl.Entry{external})
	checkFind(t, tree, "/eos/project/c/cernbox/sub", []acl.Entry{aliceRead, bobRead}, []acl.Entry{external})
}

// The deep job looks up every namespace entry, so it may want to do that with
// several goroutines. Find only reads the tree, so that is safe once the tree
// is built. Run with -race.
func TestFindIsSafeForConcurrentUse(t *testing.T) {
	tree := NewACLTree()
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox", MandatoryACLs: []acl.Entry{aliceRead}})
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox/a", MandatoryACLs: []acl.Entry{bobRead}})
	tree.Insert(&ACLNode{Path: "/eos/project/c/cernbox/b", MandatoryACLs: []acl.Entry{groupRead}})

	paths := []string{
		"/eos/project/c/cernbox",
		"/eos/project/c/cernbox/a/file.txt",
		"/eos/project/c/cernbox/b/sub/file.txt",
		"/eos/project/c/other/file.txt",
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				for _, p := range paths {
					tree.Find(p)
				}
			}
		}()
	}
	wg.Wait()

	// the lookups changed nothing
	checkFind(t, tree, "/eos/project/c/cernbox/a/file.txt", []acl.Entry{aliceRead, bobRead}, nil)
	checkFind(t, tree, "/eos/project/c/cernbox/b/sub/file.txt", []acl.Entry{aliceRead, groupRead}, nil)
}

// Matches tells whether the node covers the path: the node itself or anything
// below it. The path must be clean, which is what Find gives it.
func TestMatches(t *testing.T) {
	node := &ACLNode{Path: "/eos/project/c/cernbox"}
	tests := []struct {
		path string
		want bool
	}{
		{"/eos/project/c/cernbox", true},
		{"/eos/project/c/cernbox/sub", true},
		{"/eos/project/c/cernbox/sub/deep", true},
		{"/eos/project/c/cernboxother", false},
		{"/eos/project/c/cernbox-doc.md", false},
		{"/eos/project/c", false},
		{"/eos/project/c/other", false},
	}
	for _, tt := range tests {
		if got := node.Matches(tt.path); got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestMatchesExact(t *testing.T) {
	node := &ACLNode{Path: "/eos/project/c/cernbox"}
	tests := []struct {
		path string
		want bool
	}{
		{"/eos/project/c/cernbox", true},
		{"/eos/project/c/cernbox/sub", false},
		{"/eos/project/c", false},
	}
	for _, tt := range tests {
		if got := node.MatchesExact(tt.path); got != tt.want {
			t.Errorf("MatchesExact(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// A node covers a path only when a separator follows the prefix. A file whose
// name starts with the name of a shared folder must stay outside of it.
func TestFastHasPrefix(t *testing.T) {
	const shared = "/eos/project/c/cernbox/shared"

	tests := []struct {
		s, prefix string
		want      bool
	}{
		{"/eos/project/c/cernbox/shared/notes.md", shared, true},
		{"/eos/project/c/cernbox/shared/sub/notes.md", shared, true},
		// the file sits next to the folder, not in it
		{"/eos/project/c/cernbox/shared-doc.md", shared, false},
		{"/eos/project/c/cernbox/sharednotes", shared, false},
		{"/eos/project/c/cernbox/shared2/notes.md", shared, false},
		// the same path is not below itself
		{shared, shared, false},
		// shorter than the prefix
		{"/eos/project/c/cernbox/sha", shared, false},
		{"/eos/project/c/cernbox", shared, false},
		// the empty prefix is the root node, which covers every path
		{shared, "", true},
		// the prefix must be clean, a trailing separator finds nothing
		{"/eos/project/c/cernbox/shared/notes.md", shared + "/", false},
	}

	for _, tt := range tests {
		if got := fastHasPrefix(tt.s, tt.prefix); got != tt.want {
			t.Errorf("fastHasPrefix(%q, %q) = %v, want %v", tt.s, tt.prefix, got, tt.want)
		}
	}
}

func TestMergeACLs(t *testing.T) {
	tests := []struct {
		name string
		a, b []acl.Entry
		want []acl.Entry
	}{
		{
			name: "different entities are kept both",
			a:    []acl.Entry{aliceRead},
			b:    []acl.Entry{bobRead},
			want: []acl.Entry{aliceRead, bobRead},
		},
		{
			name: "same entity keeps the highest permission",
			a:    []acl.Entry{aliceRead},
			b:    []acl.Entry{aliceWrite},
			want: []acl.Entry{aliceWrite},
		},
		{
			name: "the highest permission wins in both directions",
			a:    []acl.Entry{aliceWrite},
			b:    []acl.Entry{aliceRead},
			want: []acl.Entry{aliceWrite},
		},
		{
			name: "same qualifier with another type is another entry",
			a:    []acl.Entry{{Type: acl.TypeUser, Qualifier: "x", Permissions: "rx"}},
			b:    []acl.Entry{{Type: acl.TypeGroup, Qualifier: "x", Permissions: "rx"}},
			want: []acl.Entry{
				{Type: acl.TypeUser, Qualifier: "x", Permissions: "rx"},
				{Type: acl.TypeGroup, Qualifier: "x", Permissions: "rx"},
			},
		},
		{
			name: "empty second set",
			a:    []acl.Entry{aliceRead},
			want: []acl.Entry{aliceRead},
		},
		{
			name: "empty first set",
			b:    []acl.Entry{aliceRead},
			want: []acl.Entry{aliceRead},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeACLs(tt.a, tt.b); !sameEntries(got, tt.want) {
				t.Errorf("mergeACLs(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// The two input sets belong to other nodes of the tree, so a merge must not
// change them.
func TestMergeACLsDoesNotChangeItsInput(t *testing.T) {
	a := []acl.Entry{aliceRead}
	b := []acl.Entry{aliceWrite, bobRead}

	mergeACLs(a, b)

	if !sameEntries(a, []acl.Entry{aliceRead}) {
		t.Errorf("first set = %v, want %v", a, []acl.Entry{aliceRead})
	}
	if !sameEntries(b, []acl.Entry{aliceWrite, bobRead}) {
		t.Errorf("second set = %v, want %v", b, []acl.Entry{aliceWrite, bobRead})
	}
}

func TestHighestPermission(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"rx", "rwx", "rwx"},
		{"rwx", "rx", "rwx"},
		{"rx", "rx", "rx"},
		{"rwx", "rwx", "rwx"},
		{"rx", "!r!w!x", "!r!w!x"},
		{"!r!w!x", "rwx", "!r!w!x"},
		// an unknown permission loses against a known one
		{"", "rx", "rx"},
		{"rx", "", "rx"},
	}
	for _, tt := range tests {
		if got := highestPermission(tt.a, tt.b); got != tt.want {
			t.Errorf("highestPermission(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}

const benchRoot = "/eos/project/c/cernbox"

// benchPaths returns one path for every node of a full tree with the given
// depth and number of children per node, parents before children.
func benchPaths(depth, children int) []string {
	paths := []string{benchRoot}
	level := []string{benchRoot}
	for range depth {
		var next []string
		for _, p := range level {
			for c := range children {
				next = append(next, fmt.Sprintf("%s/d%d", p, c))
			}
		}
		paths = append(paths, next...)
		level = next
	}
	return paths
}

// benchNodes puts one rule on every path. Every rule holds another user, so a
// deep node inherits one entry per level above it.
func benchNodes(paths []string) []*ACLNode {
	nodes := make([]*ACLNode, len(paths))
	for i, p := range paths {
		nodes[i] = &ACLNode{
			Path:          p,
			MandatoryACLs: []acl.Entry{{Type: acl.TypeUser, Qualifier: fmt.Sprintf("user%d", i), Permissions: "rx"}},
			AllowedACLs:   []acl.Entry{external},
		}
	}
	return nodes
}

// The cost of building a tree. Shares do not arrive in tree order, so the
// deepest first order is measured as well: it moves nodes and merges ACLs down
// a subtree at every step.
func BenchmarkInsert(b *testing.B) {
	shapes := []struct{ depth, children int }{
		{3, 4},
		{4, 4},
		{5, 4},
	}

	for _, s := range shapes {
		paths := benchPaths(s.depth, s.children)

		b.Run(fmt.Sprintf("%d nodes/top down", len(paths)), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				nodes := benchNodes(paths)
				tree := NewACLTree()
				b.StartTimer()

				for _, n := range nodes {
					tree.Insert(n)
				}
			}
		})

		b.Run(fmt.Sprintf("%d nodes/deepest first", len(paths)), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				nodes := benchNodes(paths)
				slices.Reverse(nodes)
				tree := NewACLTree()
				b.StartTimer()

				for _, n := range nodes {
					tree.Insert(n)
				}
			}
		})
	}
}

// The lookup, which is the operation the tree exists for.
func BenchmarkFind(b *testing.B) {
	const depth = 5
	paths := benchPaths(depth, 4)

	tree := NewACLTree()
	for _, n := range benchNodes(paths) {
		tree.Insert(n)
	}
	deepest := benchRoot + strings.Repeat("/d0", depth)

	cases := []struct{ name, path string }{
		{"top of the tree", benchRoot},
		{"deepest rule", deepest},
		{"file below the deepest rule", deepest + "/file.txt"},
		{"outside the tree", "/eos/project/c/other/file.txt"},
	}

	for _, c := range cases {
		b.Run(fmt.Sprintf("%d nodes/%s", len(paths), c.name), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tree.Find(c.path)
			}
		})
	}
}

// A space with about 1000 rules, spread over three levels of 10 children each.
// The last child of every level is the worst case, because Find scans the
// children in order.
func BenchmarkFindTenWide(b *testing.B) {
	const depth, children = 3, 10
	paths := benchPaths(depth, children)

	tree := NewACLTree()
	for _, n := range benchNodes(paths) {
		tree.Insert(n)
	}

	first := benchRoot + strings.Repeat("/d0", depth)
	last := benchRoot + strings.Repeat(fmt.Sprintf("/d%d", children-1), depth)

	cases := []struct{ name, path string }{
		{"top of the tree", benchRoot},
		{"first rule of every level", first},
		{"last rule of every level", last},
		{"file below the last rule", last + "/file.txt"},
	}

	for _, c := range cases {
		b.Run(fmt.Sprintf("%d nodes/%s", len(paths), c.name), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tree.Find(c.path)
			}
		})
	}
}

// Find scans the children of a node one by one, so a wide node is the case to
// watch: a directory with many shared children.
func BenchmarkFindWide(b *testing.B) {
	for _, children := range []int{10, 100, 1000} {
		paths := benchPaths(1, children)
		tree := NewACLTree()
		for _, n := range benchNodes(paths) {
			tree.Insert(n)
		}
		last := paths[len(paths)-1]

		b.Run(fmt.Sprintf("%d nodes/%d children", len(paths), children), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tree.Find(last)
			}
		})
	}
}
