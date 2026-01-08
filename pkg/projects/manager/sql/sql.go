// Copyright 2018-2024 CERN
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

package sql

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/projects"
	"github.com/cs3org/reva/v3/pkg/projects/manager/registry"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"gorm.io/datatypes"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	registry.Register("sql", New)
}

// Config is the configuration to use for the sql driver
// implementing the projects.Catalogue interface.
type Config struct {
	config.Database `mapstructure:",squash"`
	// CacheTTL (seconds) determines how long the list of projects will be stored in a cache
	// before a new database query is executed. The default, 0, corresponds to 60 seconds.
	CacheTTL int `mapstructure:"cache_ttl"`
}

type ProjectsManager struct {
	c     *Config
	db    *gorm.DB
	cache *ttlcache.Cache
}

const cacheKey = "projects/projectsListCache"

// Project represents a project in the DB.
type Project struct {
	gorm.Model
	SpaceID   string `gorm:"size:255;uniqueIndex:i_space_id"`
	StorageID string `gorm:"size:255"`
	Path      string
	Name      string `gorm:"size:255;uniqueIndex:i_name_archived_at"`
	// Owner of the project
	Owner string `gorm:"size:255"`
	// Readers e-group ID
	Readers string
	// Writers e-group ID
	Writers string
	// Admins e-group ID
	Admins string
	// Called description in libregraph API
	// Called subtitle in front-end
	Description string
	// Path of readme.md
	ReadmePath string
	// Path of the thumbnail file
	ThumbnailPath string
	// Set if the project is archived, i.e. not available to users in this state
	ArchivedAt datatypes.NullTime `gorm:"uniqueIndex:i_name_archived_at"`
	// Comma-seperated list of arbitrary capabilities of the project
	Capabilities string

	// For internal use:

	// Description of the use-case that was passed in the creation ticket
	UserProvidedDescription string
	// Service acount linked to the project
	ServiceAccount string
	// Comments about the project, for second / third level support
	Comments string
	// Reference to the ticket that requested the project
	SnowTicket string
	// ID of the Backup Job
	BackupJobId string
	// Initially requested capacity
	InitialCapacityBytes uint64
}

func New(ctx context.Context, m map[string]any) (projects.Catalogue, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	c.ApplyDefaults()
	if c.Engine == "" {
		return nil, fmt.Errorf("Database config: %+v", sharedconf.GetDBInfo(c.Database))
	}
	var db *gorm.DB
	var err error
	switch c.Engine {
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(c.DBName), &gorm.Config{})
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	default: // default is mysql
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	}
	if err != nil {
		return nil, errors.Wrap(err, "Failed to connect to Projects database using engine "+c.Engine)
	}

	// Migrate schemas
	err = db.AutoMigrate(&Project{})

	if err != nil {
		return nil, errors.Wrap(err, "Failed to mgirate Project schema")
	}

	cache := ttlcache.NewCache()
	if c.CacheTTL == 0 {
		c.CacheTTL = 60
	}
	cache.SetTTL(time.Duration(c.CacheTTL))
	// Even if we get a hit, of course we just want to refresh every 60 seconds
	cache.SkipTTLExtensionOnHit(true)
	mgr := &ProjectsManager{
		c:     &c,
		db:    db,
		cache: cache,
	}
	return mgr, nil
}

func (c *Config) ApplyDefaults() {
	c.Database = sharedconf.GetDBInfo(c.Database)
}

func (m *ProjectsManager) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return &provider.ListStorageSpacesResponse{
			Status: &rpcv1beta1.Status{
				Code:    rpcv1beta1.Code_CODE_UNAUTHENTICATED,
				Message: "must provide a user for listing storage spaces",
			},
		}, nil
	}

	var fetchedProjects []*Project
	// If there is a filter other than SpaceType, we don't cache
	shouldCache := !containsDriverLevelFilters(req.Filters)
	if res, err := m.cache.Get(cacheKey); shouldCache && err == nil && res != nil {
		fetchedProjects = res.([]*Project)
	} else {
		query := m.db.Model(&Project{}).Where("archived_at is null")
		query = m.appendFiltersToQuery(ctx, query, req.Filters)

		res := query.Find(&fetchedProjects)
		if res.Error != nil {
			return nil, res.Error
		}

		if shouldCache {
			m.cache.Set(cacheKey, fetchedProjects)
		}
	}

	projs := []*provider.StorageSpace{}
	for _, p := range fetchedProjects {
		if perms, ok := projectBelongsToUser(user, p); ok {
			projs = append(projs, projectToStorageSpace(p, perms))
		}
	}

	return &provider.ListStorageSpacesResponse{
		StorageSpaces: projs,
		Status: &rpcv1beta1.Status{
			Code: rpcv1beta1.Code_CODE_OK,
		},
	}, nil
}

func (m *ProjectsManager) GetStorageSpace(ctx context.Context, name string) (*provider.StorageSpace, error) {
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return nil, errors.New("must provide a user for fetching storage spaces")
	}

	fetchedProject, err := m.GetProject(ctx, name)
	if err != nil {
		return nil, err
	}

	if perms, ok := projectBelongsToUser(user, fetchedProject); ok {
		return projectToStorageSpace(fetchedProject, perms), nil
	}
	return nil, fmt.Errorf("no project named %s belonging to which user has access was found", name)
}

func (m *ProjectsManager) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	log := appctx.GetLogger(ctx)
	if req == nil || req.StorageSpace == nil || req.StorageSpace.Id == nil || req.StorageSpace.Name == "" {
		log.Error().Msg("UpdateStorageSpace called without valid request")
		return &provider.UpdateStorageSpaceResponse{
			Status: &rpcv1beta1.Status{
				Code: rpcv1beta1.Code_CODE_INVALID,
			},
		}, errors.New("Must provide an ID and name when updating a storage space")
	}
	log.Debug().Any("space", req.StorageSpace).Any("update", req.Field).Msg("Updating storage space")

	if req.Field == nil {
		return &provider.UpdateStorageSpaceResponse{
			Status: &rpcv1beta1.Status{
				Code: rpcv1beta1.Code_CODE_INVALID,
			},
		}, errors.New("No field given to update")
	}

	var res *gorm.DB

	switch req.Field.Field.(type) {
	case *provider.UpdateStorageSpaceRequest_UpdateField_Description:
		res = m.db.Model(&Project{}).
			Where("name = ?", req.StorageSpace.Name).
			Update("description", req.Field.GetDescription())
	case *provider.UpdateStorageSpaceRequest_UpdateField_Metadata:
		switch req.Field.GetMetadata().Type {
		case provider.SpaceMetadata_TYPE_README:
			res = m.db.Model(&Project{}).
				Where("name = ?", req.StorageSpace.Name).
				Update("readme_path", req.Field.GetMetadata().Id)
		case provider.SpaceMetadata_TYPE_THUMBNAIL:
			res = m.db.Model(&Project{}).
				Where("name = ?", req.StorageSpace.Name).
				Update("thumbnail_path", req.Field.GetMetadata().Id)
		}
	default:
		return nil, errors.New("Unsupported update type")
	}

	if res.Error != nil {
		log.Error().Err(res.Error).Msg("UpdateStorageSpace: database error")
		return nil, res.Error
	}

	space, err := m.GetStorageSpace(ctx, req.StorageSpace.Name)
	if err != nil {
		return nil, err
	}

	return &provider.UpdateStorageSpaceResponse{
		Status: &rpcv1beta1.Status{
			Code: rpcv1beta1.Code_CODE_OK,
		},
		StorageSpace: space,
	}, nil
}

func (m *ProjectsManager) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, errors.New("Unsupported")
}

func (m *ProjectsManager) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return nil, errors.New("Unsupported")
}

func (m *ProjectsManager) GetProject(ctx context.Context, name string) (*Project, error) {
	log := appctx.GetLogger(ctx)
	fetchedProject := &Project{}

	query := m.db.Model(&Project{}).Where("name = ?", name).Where("archived_at is null")
	res := query.First(fetchedProject)
	if res.Error != nil {
		log.Error().Err(res.Error).Msg("GetStorageSpace: database error")

		return nil, res.Error
	}

	return fetchedProject, nil
}

func projectBelongsToUser(user *userpb.User, p *Project) (*provider.ResourcePermissions, bool) {
	if user.Id.OpaqueId == p.Owner {
		return conversions.NewManagerRole().CS3ResourcePermissions(), true
	}
	if slices.Contains(user.Groups, p.Admins) {
		return conversions.NewManagerRole().CS3ResourcePermissions(), true
	}
	if slices.Contains(user.Groups, p.Writers) {
		return conversions.NewEditorRole().CS3ResourcePermissions(), true
	}
	if slices.Contains(user.Groups, p.Readers) {
		return conversions.NewViewerRole().CS3ResourcePermissions(), true
	}
	return nil, false
}

func projectToStorageSpace(p *Project, perms *provider.ResourcePermissions) *provider.StorageSpace {
	return &provider.StorageSpace{
		Id: &provider.StorageSpaceId{
			OpaqueId: spaces.EncodeStorageSpaceID(p.StorageID, p.Path),
		},
		Owner: &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: p.Owner,
			},
		},
		Name:      p.Name,
		SpaceType: spaces.SpaceTypeProject.AsString(),
		RootInfo: &provider.ResourceInfo{
			Path:          p.Path,
			PermissionSet: perms,
		},
		Description:   p.Description,
		ThumbnailId:   p.ThumbnailPath,
		ReadmeId:      p.ReadmePath,
		PermissionSet: perms,
	}
}

func (m *ProjectsManager) appendFiltersToQuery(ctx context.Context, query *gorm.DB, filters []*provider.ListStorageSpacesRequest_Filter) *gorm.DB {
	// We want to chain filters of different types with AND
	// and filters of the same type with OR
	// Therefore, we group them by type
	groupedFilters := m.GroupFiltersByType(filters)

	for filtertype, filters := range groupedFilters {
		switch filtertype {
		case provider.ListStorageSpacesRequest_Filter_TYPE_PATH:
			innerQuery := m.db
			for i, filter := range filters {
				if i == 0 {
					innerQuery = innerQuery.Where("path = ?", filter.GetPath())
				} else {
					innerQuery = innerQuery.Or("path = ?", filter.GetPath())
				}
			}
			query = query.Where(innerQuery)
		case provider.ListStorageSpacesRequest_Filter_TYPE_ID:
			innerQuery := m.db
			for i, filter := range filters {
				// ID could be both normal SpaceID and StorageSpaceID
				id := filter.GetId().OpaqueId
				if strings.Contains(id, "$") {
					parts := strings.Split(id, "$")
					if len(parts) == 2 {
						id = parts[1]
					}
				}
				if i == 0 {
					innerQuery = innerQuery.Where("space_id = ?", id)
				} else {
					innerQuery = innerQuery.Or("space_id = ?", id)
				}
			}
			query = query.Where(innerQuery)
		}
	}
	return query
}

// GroupFiltersByType groups the given filters and returns a map using the filter type as the key.
func (m *ProjectsManager) GroupFiltersByType(filters []*provider.ListStorageSpacesRequest_Filter) map[provider.ListStorageSpacesRequest_Filter_Type][]*provider.ListStorageSpacesRequest_Filter {
	grouped := make(map[provider.ListStorageSpacesRequest_Filter_Type][]*provider.ListStorageSpacesRequest_Filter)
	for _, f := range filters {
		grouped[f.Type] = append(grouped[f.Type], f)
	}
	return grouped
}

// Does the provided filter list contain filters that should be handled by the driver?
// All filters should be handled by the driver, except for the SpaceType
func containsDriverLevelFilters(filters []*provider.ListStorageSpacesRequest_Filter) bool {
	for _, f := range filters {
		if f.Type != provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE {
			return true
		}
	}
	return false
}
