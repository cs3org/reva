package indexer

import "github.com/cs3org/reva/pkg/storage/utils/indexer/index"

// typeMap stores the indexer layout at runtime.

type typeMap map[tName]typeMapping
type tName = string
type fieldName = string

type typeMapping struct {
	PKFieldName    string
	IndicesByField map[fieldName][]index.Index
}

func (m typeMap) addIndex(typeName string, pkName string, idx index.Index) {
	if val, ok := m[typeName]; ok {
		val.IndicesByField[idx.IndexBy().String()] = append(val.IndicesByField[idx.IndexBy().String()], idx)
		return
	}
	m[typeName] = typeMapping{
		PKFieldName: pkName,
		IndicesByField: map[string][]index.Index{
			idx.IndexBy().String(): {idx},
		},
	}
}
