package token

import (
	"context"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/invite/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	userPkg "github.com/cs3org/reva/pkg/user"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"math/rand"
	"time"
)

const EXPIRATION_TIME = "24h10m10s"
const TOKEN_LENGTH int = 20

func GenerateToken(expiration string, ctx context.Context) (*invitepb.InviteToken, error) {

	// Parse time duration
	duration, err := time.ParseDuration(expiration)
	if err != nil {
		return nil, errors.Wrap(err, "error parse duration")
	}

	logger := appctx.GetLogger(ctx)

	contexUser, ok := userPkg.ContextGetUser(ctx)
	if ok != false {
		return nil, errors.New("error get user data from context")
	}

	// Generate token structure
	// tokenId := generateRandomString(TOKEN_LENGTH)
	tokenId := generateUID()
	now := time.Now()
	expirationTime := now.Add(duration)

	logger.Debug().Str("tokenId", tokenId).Msg("GenerateToken")

	token := invitepb.InviteToken{
		Token: tokenId,
		UserId: &userpb.UserId{
			Idp:      contexUser.GetId().GetIdp(),
			OpaqueId: contexUser.GetId().GetOpaqueId(),
		},
		Expiration: &typesv1beta1.Timestamp{
			Seconds: uint64(expirationTime.Unix()),
			Nanos:   0,
		},
	}

	return &token, nil
}

func generateUID() string {
	return uuid.New().String()
}

func generateRandomString(n int) string {
	var l = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = l[rand.Intn(len(l))]
	}
	return string(b)
}
