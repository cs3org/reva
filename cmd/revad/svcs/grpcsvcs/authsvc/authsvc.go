package authsvc

import (
	"context"

	"github.com/cernbox/reva/cmd/revad/grpcserver"
	"github.com/cernbox/reva/pkg/auth/manager/registry"
	tokenmgr "github.com/cernbox/reva/pkg/token/manager/registry"
	usermgr "github.com/cernbox/reva/pkg/user/manager/registry"
	"google.golang.org/grpc"

	"github.com/cernbox/reva/pkg/auth"
	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/token"
	"github.com/cernbox/reva/pkg/user"

	authv0alphapb "github.com/cernbox/go-cs3apis/cs3/auth/v0alpha"
	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"

	"github.com/mitchellh/mapstructure"
)

var logger = log.New("authsvc")
var errors = err.New("authsvc")
var ctx = context.Background()

func init() {
	grpcserver.Register("authsvc", New)
}

type config struct {
	AuthManager  map[string]interface{} `mapstructure:"auth_manager"`
	TokenManager map[string]interface{} `mapstructure:"token_manager"`
	UserManager  map[string]interface{} `mapstructure:"user_manager"`
}

type authManagerConfig struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

type tokenManagerConfig struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

type userManagerConfig struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil

}

func getUserManager(m map[string]interface{}) (user.Manager, error) {
	c := &userManagerConfig{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}

	if f, ok := usermgr.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}

	return nil, errors.Errorf("driver %s not found for user manager", c.Driver)
}

func getAuthManager(m map[string]interface{}) (auth.Manager, error) {
	c := &authManagerConfig{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}

	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}

	return nil, errors.Errorf("driver %s not found for auth manager", c.Driver)
}

func getTokenManager(m map[string]interface{}) (token.Manager, error) {
	c := &tokenManagerConfig{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}

	if f, ok := tokenmgr.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}

	return nil, errors.Errorf("driver %s not found for token manager", c.Driver)
}

// New returns a new AuthServiceServer.
func New(m map[string]interface{}, ss *grpc.Server) error {
	c, err := parseConfig(m)
	if err != nil {
		return err
	}

	authManager, err := getAuthManager(c.AuthManager)
	if err != nil {
		return err
	}

	tokenManager, err := getTokenManager(c.TokenManager)
	if err != nil {
		return err
	}

	userManager, err := getUserManager(c.UserManager)
	if err != nil {
		return err
	}

	svc := &service{authmgr: authManager, tokenmgr: tokenManager, usermgr: userManager}
	authv0alphapb.RegisterAuthServiceServer(ss, svc)
	return nil
}

type service struct {
	authmgr  auth.Manager
	tokenmgr token.Manager
	usermgr  user.Manager
}

func (s *service) GenerateAccessToken(ctx context.Context, req *authv0alphapb.GenerateAccessTokenRequest) (*authv0alphapb.GenerateAccessTokenResponse, error) {
	username := req.GetUsername()
	password := req.GetPassword()

	err := s.authmgr.Authenticate(ctx, username, password)
	if err != nil {
		err = errors.Wrap(err, "error authenticating user")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_UNAUTHENTICATED}
		res := &authv0alphapb.GenerateAccessTokenResponse{Status: status}
		return res, nil
	}

	user, err := s.usermgr.GetUser(ctx, username)
	if err != nil {
		err = errors.Wrap(err, "error getting user information")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_UNAUTHENTICATED}
		res := &authv0alphapb.GenerateAccessTokenResponse{Status: status}
		return res, nil
	}

	claims := token.Claims{
		"username":     user.Username,
		"groups":       user.Groups,
		"mail":         user.Mail,
		"display_name": user.DisplayName,
	}

	accessToken, err := s.tokenmgr.ForgeToken(ctx, claims)
	if err != nil {
		err = errors.Wrap(err, "error creating access token")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_UNAUTHENTICATED}
		res := &authv0alphapb.GenerateAccessTokenResponse{Status: status}
		return res, nil
	}

	logger.Printf(ctx, "user %s authenticated", user.Username)
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &authv0alphapb.GenerateAccessTokenResponse{Status: status, AccessToken: accessToken}
	return res, nil

}

func (s *service) WhoAmI(ctx context.Context, req *authv0alphapb.WhoAmIRequest) (*authv0alphapb.WhoAmIResponse, error) {
	token := req.AccessToken
	claims, err := s.tokenmgr.DismantleToken(ctx, token)
	if err != nil {
		err = errors.Wrap(err, "error dismantling access token")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_UNAUTHENTICATED}
		res := &authv0alphapb.WhoAmIResponse{Status: status}
		return res, nil
	}

	up := &struct {
		Username    string   `mapstructure:"username"`
		DisplayName string   `mapstructure:"display_name"`
		Mail        string   `mapstructure:"mail"`
		Groups      []string `mapstructure:"groups"`
	}{}

	if err := mapstructure.Decode(claims, up); err != nil {
		err = errors.Wrap(err, "error parsing token claims")
		logger.Error(ctx, err)
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_UNAUTHENTICATED}
		res := &authv0alphapb.WhoAmIResponse{Status: status}
		return res, nil
	}

	user := &authv0alphapb.User{
		Username:    up.Username,
		DisplayName: up.DisplayName,
		Mail:        up.Mail,
		Groups:      up.Groups,
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &authv0alphapb.WhoAmIResponse{Status: status, User: user}
	return res, nil
}

/*
// Override the Auth function to avoid checking the bearer token for this service
// https://github.com/grpc-ecosystem/go-grpc-middleware/tree/master/auth#type-serviceauthfuncoverride
func (s *service) AuthFuncOverride(ctx context.Context, fullMethodNauthmgre string) (context.Context, error) {
	return ctx, nil
}
*/
