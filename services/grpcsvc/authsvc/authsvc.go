package authsvc

import (
	"context"

	"github.com/cernbox/reva/pkg/auth"
	"github.com/cernbox/reva/pkg/auth/manager/demo"
	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/token"
	"github.com/cernbox/reva/pkg/token/manager/jwt"
	"github.com/cernbox/reva/pkg/user"
	usrmgrdemo "github.com/cernbox/reva/pkg/user/manager/demo"

	authv0alphapb "github.com/cernbox/go-cs3apis/cs3/auth/v0alpha"
	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"

	"github.com/mitchellh/mapstructure"
)

var logger = log.New("authsvc")
var errors = err.New("authsvc")
var ctx = context.Background()

type config struct {
	AuthManager  map[string]interface{} `mapstructure:"auth_manager"`
	TokenManager map[string]interface{} `mapstructure:"token_manager"`
	UserManager  map[string]interface{} `mapstructure:"user_manager"`
}

type authManagerConfig struct {
	Driver string                 `mapstructure:"driver"`
	Demo   map[string]interface{} `mapstructure:"demo"`
	LDAP   map[string]interface{} `mapstructure:"ldap"`
}

type tokenManagerConfig struct {
	Driver string                 `mapstructure:"driver"`
	JWT    map[string]interface{} `mapstructure:"jwt"`
}

type userManagerConfig struct {
	Driver string                 `mapstructure:"driver"`
	Demo   map[string]interface{} `mapstructure:"demo"`
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

	switch c.Driver {
	case "demo":
		mgr, err := usrmgrdemo.New(c.Demo)
		if err != nil {
			return nil, errors.Wrap(err, "unable to create demo user manager")
		}
		return mgr, nil
	case "":
		return nil, errors.Errorf("driver for user manager is empty")

	default:
		return nil, errors.Errorf("driver %s not found for user manager", c.Driver)
	}
}

func getAuthManager(m map[string]interface{}) (auth.Manager, error) {
	c := &authManagerConfig{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}

	switch c.Driver {
	case "demo":
		mgr, err := demo.New(c.Demo)
		if err != nil {
			return nil, errors.Wrap(err, "unable to create demo auth manager")
		}
		return mgr, nil
	case "":
		return nil, errors.Errorf("driver for auth manager is empty")

	default:
		return nil, errors.Errorf("driver %s not found for auth manager", c.Driver)
	}
}

func getTokenManager(m map[string]interface{}) (token.Manager, error) {
	c := &tokenManagerConfig{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}

	switch c.Driver {
	case "jwt":
		mgr, err := jwt.New(c.JWT)
		if err != nil {
			return nil, errors.Wrap(err, "unable to create jwt token manager")
		}
		return mgr, nil
	case "":
		return nil, errors.Errorf("driver for token manager is empty")

	default:
		return nil, errors.Errorf("driver %s not found for token manager", c.Driver)
	}
}

func New(m map[string]interface{}) (authv0alphapb.AuthServiceServer, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	authManager, err := getAuthManager(c.AuthManager)
	if err != nil {
		return nil, err
	}

	tokenManager, err := getTokenManager(c.TokenManager)
	if err != nil {
		return nil, err
	}

	userManager, err := getUserManager(c.UserManager)
	if err != nil {
		return nil, err
	}

	svc := &service{authmgr: authManager, tokenmgr: tokenManager, usermgr: userManager}
	return svc, nil

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
func (s *service) ForgeUserToken(ctx context.Context, req *api.ForgeUserTokenReq) (*api.TokenResponse, error) {
	l := ctx_zap.Extract(ctx)
	user, err := s.authmgr.Authenticate(ctx, req.ClientId, req.ClientSecret)
	if err != nil {
		l.Error("", zap.Error(err))
		return nil, err
	}

	token, err := s.tokenmgr.ForgeUserToken(ctx, user)
	if err != nil {
		l.Error("", zap.Error(err))
		return nil, err
	}
	tokenResponse := &api.TokenResponse{Token: token}
	return tokenResponse, nil
}

func (s *service) DismantleUserToken(ctx context.Context, req *api.TokenReq) (*api.UserResponse, error) {
	l := ctx_zap.Extract(ctx)
	token := req.Token
	u, err := s.tokenmgr.DismantleUserToken(ctx, token)
	if err != nil {
		l.Warn("token invalid", zap.Error(err))
		res := &api.UserResponse{Status: api.StatusCode_TOKEN_INVALID}
		return res, nil
		//return nil, api.NewError(api.TokenInvalidErrorCode).WithMessage(err.Error())
	}
	userRes := &api.UserResponse{User: u}
	return userRes, nil
}

func (s *service) ForgePublicLinkToken(ctx context.Context, req *api.ForgePublicLinkTokenReq) (*api.TokenResponse, error) {
	l := ctx_zap.Extract(ctx)
	pl, err := s.lm.AuthenticatePublicLink(ctx, req.Token, req.Password)
	if err != nil {
		if api.IsErrorCode(err, api.PublicLinkInvalidPasswordErrorCode) {
			return &api.TokenResponse{Status: api.StatusCode_PUBLIC_LINK_INVALID_PASSWORD}, nil
		}
		l.Error("", zap.Error(err))
		return nil, err
	}

	token, err := s.tokenmgr.ForgePublicLinkToken(ctx, pl)
	if err != nil {
		l.Warn("", zap.Error(err))
		return nil, err
	}
	tokenResponse := &api.TokenResponse{Token: token}
	return tokenResponse, nil
}

func (s *service) DismantlePublicLinkToken(ctx context.Context, req *api.TokenReq) (*api.PublicLinkResponse, error) {
	l := ctx_zap.Extract(ctx)
	token := req.Token
	u, err := s.tokenmgr.DismantlePublicLinkToken(ctx, token)
	if err != nil {
		l.Error("token invalid", zap.Error(err))
		return nil, api.NewError(api.TokenInvalidErrorCode).WithMessage(err.Error())
	}
	userRes := &api.PublicLinkResponse{PublicLink: u}
	return userRes, nil
}

// Override the Auth function to avoid checking the bearer token for this service
// https://github.com/grpc-ecosystem/go-grpc-middleware/tree/master/auth#type-serviceauthfuncoverride
func (s *service) AuthFuncOverride(ctx context.Context, fullMethodNauthmgre string) (context.Context, error) {
	return ctx, nil
}
*/
