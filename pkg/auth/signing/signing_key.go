package signing

import (
	"encoding/hex"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"

	"golang.org/x/crypto/argon2"
)

func DeriveSigningKey(u *user.User, secret, date string) []byte {
	bytesKey := argon2.Key([]byte(secret), []byte(u.Username+date), 3, 32*1024, 4, 32)
	hexKey := hex.EncodeToString(bytesKey)
	return []byte(hexKey)
}
