package ocis

// NodeInfo allows referencing a node by id and optionally a relative path
type NodeInfo struct {
	ParentID string
	ID       string
	Name     string
	Exists   bool
}

// BecomeParent rewrites the internal state to point to the parent id
func (n *NodeInfo) BecomeParent() {
	n.ID = n.ParentID
	n.ParentID = ""
	n.Name = ""
	n.Exists = false
}
