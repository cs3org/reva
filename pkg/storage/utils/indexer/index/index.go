package index

import "github.com/cs3org/reva/pkg/storage/utils/indexer/option"

// Index can be implemented to create new indexer-strategies. See Unique for example.
// Each indexer implementation is bound to one data-column (IndexBy) and a data-type (TypeName)
type Index interface {
	Init() error
	Lookup(v string) ([]string, error)
	Add(id, v string) (string, error)
	Remove(id string, v string) error
	Update(id, oldV, newV string) error
	Search(pattern string) ([]string, error)
	CaseInsensitive() bool
	IndexBy() option.IndexBy
	TypeName() string
	FilesDir() string
	Delete() error // Delete deletes the index folder from its storage.
}
