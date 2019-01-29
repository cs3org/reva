package jwt

import (
	"context"

	"github.com/cernbox/reva/pkg/token"

	"github.com/dgrijalva/jwt-go"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type config struct {
	Secret string `mapstructure:"secret"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New returns an implementation of the token manager that uses JWT as tokens.
func New(m map[string]interface{}) (token.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	return &manager{secret: c.Secret}, nil
}

type manager struct {
	secret string
}

func (tm *manager) ForgeToken(ctx context.Context, claims token.Claims) (string, error) {
	jwtClaims := jwt.MapClaims(claims)
	token := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), jwtClaims)
	signedToken, err := token.SignedString([]byte(tm.secret))
	if err != nil {
		return "", errors.Wrapf(err, "jwt: error signing token with claims=%+v", jwtClaims)
	}
	return signedToken, nil
}

func (tm *manager) DismantleToken(ctx context.Context, t string) (token.Claims, error) {
	jwtToken, err := jwt.Parse(t, func(token *jwt.Token) (interface{}, error) {
		return []byte(tm.secret), nil
	})

	if err != nil {
		return nil, errors.Wrap(err, "jwt: error parsing token")
	}
	if !jwtToken.Valid {
		return nil, errors.Wrap(err, "jwt: token is invalid")

	}

	jwtClaims := jwtToken.Claims.(jwt.MapClaims)
	claims := token.Claims(jwtClaims)
	return claims, nil

}

/*
func (tm *manager) ForgeUserToken(ctx context.Context, user *api.User) (string, error) {
	token := jwt.New(jwt.GetSigningMethod("HS256"))
	claims := token.Claims.(jwt.MapClaims)
	claims["account_id"] = user.AccountId
	claims["display_name"] = user.DisplayName
	claims["groups"] = user.Groups
	claims["exp"] = time.Now().Add(time.Second * time.Duration(3600))
	tokenString, err := token.SignedString([]byte(tm.secret))
	if err != nil {
		l.Error("", zap.Error(err))
		return "", err
	}
	return tokenString, nil
}

func (tm *manager) DismantleUserToken(ctx context.Context, token string) (*api.User, error) {
	l := ctx_zap.Extract(ctx)
	rawToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		return []byte(tm.secret), nil
	})
	if err != nil {
		l.Error("invalid token", zap.Error(err), zap.String("token", token))
		return nil, err
	}
	if !rawToken.Valid {
		l.Error("invalid token", zap.Error(err), zap.String("token", token))
		return nil, err

	}

	claims := rawToken.Claims.(jwt.MapClaims)
	accountID, ok := claims["account_id"].(string)
	if !ok {
		return nil, errors.New("account_id claim is not a string")
	}

	displayName, _ := claims["display_name"].(string) // no displayname is not an error

	rawGroups, ok := claims["groups"].([]interface{})
	if !ok {
		return nil, errors.New("groups claim is not a []interface{}")
	}
	groups := []string{}
	for _, g := range rawGroups {
		group, ok := g.(string)
		if !ok {
			err := errors.New(fmt.Sprintf("group %+v can not be casted to string", g))
			l.Error("", zap.Error(err))
			return nil, err
		}
		groups = append(groups, group)
	}

	user := &api.User{
		AccountId:   accountID,
		Groups:      groups,
		DisplayName: displayName,
	}
	return user, nil
}

func (tm *manager) ForgePublicLinkToken(ctx context.Context, pl *api.PublicLink) (string, error) {
	l := ctx_zap.Extract(ctx)
	token := jwt.New(jwt.GetSigningMethod("HS256"))
	claims := token.Claims.(jwt.MapClaims)
	claims["token"] = pl.Token
	claims["owner"] = pl.OwnerId
	claims["id"] = pl.Id
	claims["path"] = pl.Path
	claims["protected"] = pl.Protected
	claims["expires"] = pl.Expires
	claims["read_only"] = pl.ReadOnly
	claims["mtime"] = pl.Mtime
	claims["item_type"] = pl.ItemType
	claims["share_name"] = pl.Name
	claims["exp"] = time.Now().Add(time.Second * time.Duration(3600))
	tokenString, err := token.SignedString([]byte(tm.secret))
	if err != nil {
		l.Error("", zap.Error(err))
		return "", err
	}
	return tokenString, nil
}

func (tm *manager) DismantlePublicLinkToken(ctx context.Context, token string) (*api.PublicLink, error) {
	l := ctx_zap.Extract(ctx)
	rawToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		return []byte(tm.secret), nil
	})
	if err != nil {
		l.Error("invalid token", zap.Error(err), zap.String("token", token))
		return nil, err
	}
	if !rawToken.Valid {
		l.Error("invalid token", zap.Error(err), zap.String("token", token))
		return nil, err

	}

	//
	//"exp": "2018-07-24T10:11:11.827901148+02:00",
	//	"expires": 0,
	//	"id": "103",
	//	"item_type": 0,
	//	"mtime": 1532362779,
	//	"owner": "gonzalhu",
	//	"path": "oldhome:22510091102060544",
	//	"protected": false,
	//	"read_only": true,
	//	"token": "fgDsc2WD8F2qNfH"
	//
	claims := rawToken.Claims.(jwt.MapClaims)
	token, ok := claims["token"].(string)
	if !ok {
		return nil, errors.New("token claim is not a string")
	}
	owner, ok := claims["owner"].(string)
	if !ok {
		return nil, errors.New("owner claim is not a string")
	}
	readOnly, ok := claims["read_only"].(bool)
	if !ok {
		return nil, errors.New("read_only claim is not a bool")
	}
	path, ok := claims["path"].(string)
	if !ok {
		return nil, errors.New("path claim is not a string")
	}
	protected, ok := claims["protected"].(bool)
	if !ok {
		return nil, errors.New("protected claim is not a bool")
	}
	mtime, ok := claims["mtime"].(float64)
	if !ok {
		return nil, errors.New("mtime claim is not a float64")
	}
	itemType, ok := claims["item_type"].(float64)
	if !ok {
		return nil, errors.New("item_type claim is not a float64")
	}
	shareName, ok := claims["share_name"].(string)
	if !ok {
		return nil, errors.New("share_name claim is not a string")
	}

	pl := &api.PublicLink{
		Token:     token,
		OwnerId:   owner,
		ReadOnly:  readOnly,
		Path:      path,
		Protected: protected,
		Mtime:     uint64(mtime),
		ItemType:  api.PublicLink_ItemType(itemType),
		Name:      shareName,
	}
	return pl, nil
}
*/
