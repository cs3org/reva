// Copyright 2018-2022 CERN
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

// Package indexer provides symlink-based indexer for on-disk document-directories.
package indexer

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/CiscoM31/godata"
	"github.com/iancoleman/strcase"

	errorspkg "github.com/cs3org/reva/pkg/storage/utils/indexer/errors"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/index"
	"github.com/cs3org/reva/pkg/storage/utils/indexer/option"
	"github.com/cs3org/reva/pkg/storage/utils/metadata"
	"github.com/cs3org/reva/pkg/storage/utils/sync"
)

// Indexer is a facade to configure and query over multiple indices.
type Indexer struct {
	storage metadata.Storage
	indices typeMap
	mu      sync.NamedRWMutex
}

// IdxAddResult represents the result of an Add call on an index
type IdxAddResult struct {
	Field, Value string
}

// CreateIndexer creates a new Indexer.
func CreateIndexer(storage metadata.Storage) *Indexer {
	return &Indexer{
		storage: storage,
		indices: typeMap{},
		mu:      sync.NewNamedRWMutex(),
	}
}

// Reset takes care of deleting all indices from storage and from the internal map of indices
func (i *Indexer) Reset() error {
	for j := range i.indices {
		for _, indices := range i.indices[j].IndicesByField {
			for _, idx := range indices {
				err := idx.Delete()
				if err != nil {
					return err
				}
			}
		}
		delete(i.indices, j)
	}

	return nil
}

// AddIndex adds a new index to the indexer receiver.
func (i *Indexer) AddIndex(t interface{}, indexBy option.IndexBy, pkName, entityDirName, indexType string, bound *option.Bound, caseInsensitive bool) error {
	var idx index.Index

	var f func(metadata.Storage, ...option.Option) index.Index
	switch indexType {
	case "unique":
		f = index.NewUniqueIndexWithOptions
	case "non_unique":
		f = index.NewNonUniqueIndexWithOptions
	case "autoincrement":
		f = index.NewAutoincrementIndex
	default:
		return fmt.Errorf("invalid index type: %s", indexType)
	}
	idx = f(
		i.storage,
		option.CaseInsensitive(caseInsensitive),
		option.WithBounds(bound),
		option.WithIndexBy(indexBy),
		option.WithTypeName(getTypeFQN(t)),
	)

	i.indices.addIndex(getTypeFQN(t), pkName, idx)
	return idx.Init()
}

// Add a new entry to the indexer
func (i *Indexer) Add(t interface{}) ([]IdxAddResult, error) {
	typeName := getTypeFQN(t)

	i.mu.Lock(typeName)
	defer i.mu.Unlock(typeName)

	var results []IdxAddResult
	if fields, ok := i.indices[typeName]; ok {
		for _, indices := range fields.IndicesByField {
			for _, idx := range indices {
				pkVal, err := valueOf(t, option.IndexByField(fields.PKFieldName))
				if err != nil {
					return []IdxAddResult{}, err
				}
				idxByVal, err := valueOf(t, idx.IndexBy())
				if err != nil {
					return []IdxAddResult{}, err
				}
				value, err := idx.Add(pkVal, idxByVal)
				if err != nil {
					return []IdxAddResult{}, err
				}
				if value == "" {
					continue
				}
				results = append(results, IdxAddResult{Field: idx.IndexBy().String(), Value: value})
			}
		}
	}

	return results, nil
}

// FindBy finds a value on an index by field and value.
func (i *Indexer) FindBy(t interface{}, findBy, val string) ([]string, error) {
	typeName := getTypeFQN(t)

	i.mu.RLock(typeName)
	defer i.mu.RUnlock(typeName)

	resultPaths := make([]string, 0)
	if fields, ok := i.indices[typeName]; ok {
		for _, idx := range fields.IndicesByField[strcase.ToCamel(findBy)] {
			idxVal := val
			res, err := idx.Lookup(idxVal)
			if err != nil {
				if errorspkg.IsNotFoundErr(err) {
					continue
				}

				if err != nil {
					return nil, err
				}
			}

			resultPaths = append(resultPaths, res...)

		}
	}

	result := make([]string, 0, len(resultPaths))
	for _, v := range resultPaths {
		result = append(result, path.Base(v))
	}

	return result, nil
}

// Delete deletes all indexed fields of a given type t on the Indexer.
func (i *Indexer) Delete(t interface{}) error {
	typeName := getTypeFQN(t)

	i.mu.Lock(typeName)
	defer i.mu.Unlock(typeName)

	if fields, ok := i.indices[typeName]; ok {
		for _, indices := range fields.IndicesByField {
			for _, idx := range indices {
				pkVal, err := valueOf(t, option.IndexByField(fields.PKFieldName))
				if err != nil {
					return err
				}
				idxByVal, err := valueOf(t, idx.IndexBy())
				if err != nil {
					return err
				}
				if err := idx.Remove(pkVal, idxByVal); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// FindByPartial allows for glob search across all indexes.
func (i *Indexer) FindByPartial(t interface{}, field string, pattern string) ([]string, error) {
	typeName := getTypeFQN(t)

	i.mu.RLock(typeName)
	defer i.mu.RUnlock(typeName)

	resultPaths := make([]string, 0)
	if fields, ok := i.indices[typeName]; ok {
		for _, idx := range fields.IndicesByField[strcase.ToCamel(field)] {
			res, err := idx.Search(pattern)
			if err != nil {
				if errorspkg.IsNotFoundErr(err) {
					continue
				}

				if err != nil {
					return nil, err
				}
			}

			resultPaths = append(resultPaths, res...)

		}
	}

	result := make([]string, 0, len(resultPaths))
	for _, v := range resultPaths {
		result = append(result, path.Base(v))
	}

	return result, nil

}

// Update updates all indexes on a value <from> to a value <to>.
func (i *Indexer) Update(from, to interface{}) error {
	typeNameFrom := getTypeFQN(from)

	i.mu.Lock(typeNameFrom)
	defer i.mu.Unlock(typeNameFrom)

	if typeNameTo := getTypeFQN(to); typeNameFrom != typeNameTo {
		return fmt.Errorf("update types do not match: from %v to %v", typeNameFrom, typeNameTo)
	}

	if fields, ok := i.indices[typeNameFrom]; ok {
		for fName, indices := range fields.IndicesByField {
			oldV, err := valueOf(from, option.IndexByField(fName))
			if err != nil {
				return err
			}
			newV, err := valueOf(to, option.IndexByField(fName))
			if err != nil {
				return err
			}
			pkVal, err := valueOf(from, option.IndexByField(fields.PKFieldName))
			if err != nil {
				return err
			}
			for _, idx := range indices {
				if oldV == newV {
					continue
				}
				if oldV == "" {
					if _, err := idx.Add(pkVal, newV); err != nil {
						return err
					}
					continue
				}
				if newV == "" {
					if err := idx.Remove(pkVal, oldV); err != nil {
						return err
					}
					continue
				}
				if err := idx.Update(pkVal, oldV, newV); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// Query parses an OData query into something our indexer.Index understands and resolves it.
func (i *Indexer) Query(ctx context.Context, t interface{}, q string) ([]string, error) {
	query, err := godata.ParseFilterString(ctx, q)
	if err != nil {
		return nil, err
	}

	tree := newQueryTree()
	if err := buildTreeFromOdataQuery(query.Tree, &tree); err != nil {
		return nil, err
	}

	results := make([]string, 0)
	if err := i.resolveTree(t, &tree, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// t is used to infer the indexed field names. When building an index search query, field names have to respect Golang
// conventions and be in PascalCase. For a better overview on this contemplate reading the reflection package under the
// indexer directory. Traversal of the tree happens in a pre-order fashion.
// TODO implement logic for `and` operators.
func (i *Indexer) resolveTree(t interface{}, tree *queryTree, partials *[]string) error {
	if partials == nil {
		return errors.New("return value cannot be nil: partials")
	}

	if tree.left != nil {
		_ = i.resolveTree(t, tree.left, partials)
	}

	if tree.right != nil {
		_ = i.resolveTree(t, tree.right, partials)
	}

	// by the time we're here we reached a leaf node.
	if tree.token != nil {
		switch tree.token.filterType {
		case "FindBy":
			operand, err := sanitizeInput(tree.token.operands)
			if err != nil {
				return err
			}

			r, err := i.FindBy(t, operand.field, operand.value)
			if err != nil {
				return err
			}

			*partials = append(*partials, r...)
		case "FindByPartial":
			operand, err := sanitizeInput(tree.token.operands)
			if err != nil {
				return err
			}

			r, err := i.FindByPartial(t, operand.field, fmt.Sprintf("%v*", operand.value))
			if err != nil {
				return err
			}

			*partials = append(*partials, r...)
		default:
			return fmt.Errorf("unsupported filter: %v", tree.token.filterType)
		}
	}

	*partials = dedup(*partials)
	return nil
}

type indexerTuple struct {
	field, value string
}

// sanitizeInput returns a tuple of fieldName + value to be applied on indexer.Index filters.
func sanitizeInput(operands []string) (*indexerTuple, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("invalid number of operands for filter function: got %v expected 2", len(operands))
	}

	// field names are Go public types and by design they are in PascalCase, therefore we need to adhere to this rules.
	// for further information on this have a look at the reflection package.
	f := strcase.ToCamel(operands[0])

	// remove single quotes from value.
	v := strings.ReplaceAll(operands[1], "'", "")
	return &indexerTuple{
		field: f,
		value: v,
	}, nil
}

// buildTreeFromOdataQuery builds an indexer.queryTree out of a GOData ParseNode. The purpose of this intermediate tree
// is to transform godata operators and functions into supported operations on our index. At the time of this writing
// we only support `FindBy` and `FindByPartial` queries as these are the only implemented filters on indexer.Index(es).
func buildTreeFromOdataQuery(root *godata.ParseNode, tree *queryTree) error {
	if root.Token.Type == godata.ExpressionTokenFunc { // i.e "startswith", "contains"
		switch root.Token.Value {
		case "startswith":
			token := token{
				operator:   root.Token.Value,
				filterType: "FindByPartial",
				// TODO sanitize the number of operands it the expected one.
				operands: []string{
					root.Children[0].Token.Value, // field name, i.e: Name
					root.Children[1].Token.Value, // field value, i.e: Jac
				},
			}

			tree.insert(&token)
		default:
			return errors.New("operation not supported")
		}
	}

	if root.Token.Type == godata.ExpressionTokenLogical {
		switch root.Token.Value {
		case "or":
			tree.insert(&token{operator: root.Token.Value})
			for _, child := range root.Children {
				if err := buildTreeFromOdataQuery(child, tree.left); err != nil {
					return err
				}
			}
		case "eq":
			tree.insert(&token{
				operator:   root.Token.Value,
				filterType: "FindBy",
				operands: []string{
					root.Children[0].Token.Value,
					root.Children[1].Token.Value,
				},
			})
			for _, child := range root.Children {
				if err := buildTreeFromOdataQuery(child, tree.left); err != nil {
					return err
				}
			}
		default:
			return errors.New("operator not supported")
		}
	}
	return nil
}
