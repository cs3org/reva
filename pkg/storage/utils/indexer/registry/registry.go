package registry

import (
	"github.com/cs3org/reva/pkg/storage/utils/indexer/index"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/option"
)

// IndexConstructor is a constructor function for creating index.Index.
type IndexConstructor func(o ...option.Option) index.Index

// IndexConstructorRegistry undocumented.
var IndexConstructorRegistry = map[string]map[string]IndexConstructor{
	"disk": {},
	"cs3":  {},
}
