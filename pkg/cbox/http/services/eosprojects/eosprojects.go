package eosprojects

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/juliangruber/go-intersect"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("eosprojects", New)
}

type eosProj struct {
	log *zerolog.Logger
	c   *config
	db  *sql.DB
}

type config struct {
	Username              string `mapstructure:"username"`
	Password              string `mapstructure:"password"`
	Host                  string `mapstructure:"host"`
	Port                  int    `mapstructure:"port"`
	Name                  string `mapstructure:"name"`
	Table                 string `mapstructure:"table"`
	Prefix                string `mapstructure:"db"`
	GatewaySvc            string `mapstructure:"gateway_svc"`
	SkipUserGroupsInToken bool   `mapstructure:"skip_user_groups_in_token"`
}

type project struct {
	Name        string `json:"name,omitempty"`
	Path        string `json:"path,omitempty"`
	Permissions string `json:"permissions,omitempty"`
}

var projectRegex = regexp.MustCompile(`^cernbox-project-(?P<Name>.+)-(?P<Permissions>admins|writers|readers)\z`)

func parseConfig(c map[string]interface{}) (*config, error) {
	conf := &config{}
	err := mapstructure.Decode(c, conf)
	if err != nil {
		return nil, errors.Wrap(err, "error deconding config")
	}
	return conf, nil
}

func (c *config) init() {
	if c.Prefix == "" {
		c.Prefix = "projects"
	}

	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)

	c.SkipUserGroupsInToken = c.SkipUserGroupsInToken || sharedconf.SkipUserGroupsInToken()
}

func New(conf map[string]interface{}, log *zerolog.Logger) (global.Service, error) {

	c, err := parseConfig(conf)
	if err != nil {
		return nil, err
	}

	c.init()

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", c.Username, c.Password, c.Host, c.Port, c.Name))
	if err != nil {
		return nil, errors.Wrap(err, "error creating open sql connection")
	}

	e := &eosProj{
		log: log,
		c:   c,
		db:  db,
	}

	return e, nil
}

func (e *eosProj) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		projects, err := e.getProjects(ctx)
		if err != nil {
			if errors.Is(err, errtypes.UserRequired("")) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}

		data, err := encodeProjectsInJSON(projects)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Write(data)
	})
}

func encodeProjectsInJSON(p []*project) ([]byte, error) {
	out := struct {
		Projects []*project `json:"projects,omitempty"`
	}{
		Projects: p,
	}
	return json.Marshal(out)
}

func (e *eosProj) Prefix() string {
	return e.c.Prefix
}

func (e *eosProj) Close() error {
	return e.db.Close()
}

func (e *eosProj) Unprotected() []string {
	return nil
}

// Prefix     string `mapstructure:"prefix"`
// GatewaySvc string `mapstructure:"gatewaysvc"`
// Insecure   bool   `mapstructure:"insecure" docs:"false;Whether to skip certificate checks when sending requests."`

func (e *eosProj) getProjects(ctx context.Context) ([]*project, error) {
	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, errtypes.UserRequired("")
	}

	groups := user.Groups
	if e.c.SkipUserGroupsInToken {
		var err error
		groups, err = e.getUserGroups(ctx, user)
		if err != nil {
			return nil, errors.Wrap(err, "error getting user groups")
		}
	}

	userProjects := make(map[string]string)
	var userProjectsKeys []string

	for _, group := range groups {
		match := projectRegex.FindStringSubmatch(group)
		if match != nil {
			if userProjects[match[1]] == "" {
				userProjectsKeys = append(userProjectsKeys, match[1])
			}
			userProjects[match[1]] = getHigherPermission(userProjects[match[1]], match[2])
		}
	}

	if len(userProjectsKeys) == 0 {
		// User has no projects... lets bail
		return []*project{}, nil
	}

	var dbProjects []string
	dbProjectsPaths := make(map[string]string)
	query := fmt.Sprintf("SELECT project_name, eos_relative_path FROM %s", e.c.Table)
	results, err := e.db.Query(query)
	if err != nil {
		return nil, errors.Wrap(err, "error getting projects from db")
	}

	for results.Next() {
		var name string
		var path string
		err = results.Scan(&name, &path)
		if err != nil {
			return nil, errors.Wrap(err, "error scanning rows from db")
		}
		dbProjects = append(dbProjects, name)
		dbProjectsPaths[name] = path
	}

	validProjects := intersect.Simple(dbProjects, userProjectsKeys)

	var projects []*project
	for _, p := range validProjects {
		name := p.(string)
		permissions := userProjects[name]
		projects = append(projects, &project{
			Name:        name,
			Path:        fmt.Sprintf("/eos/project/%s", dbProjectsPaths[name]),
			Permissions: permissions[:len(permissions)-1],
		})
	}

	return projects, nil
}

func (e *eosProj) getUserGroups(ctx context.Context, user *userpb.User) ([]string, error) {
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(e.c.GatewaySvc))
	if err != nil {
		return nil, err
	}

	res, err := client.GetUserGroups(context.Background(), &userpb.GetUserGroupsRequest{UserId: user.Id})
	if err != nil {
		return nil, err
	}

	return res.Groups, nil
}

var permissionsLevel = map[string]int{
	"admins":  1,
	"writers": 2,
	"readers": 3,
}

func getHigherPermission(perm1, perm2 string) string {
	if perm1 == "" {
		return perm2
	}
	if permissionsLevel[perm1] < permissionsLevel[perm2] {
		return perm1
	}
	return perm2
}
