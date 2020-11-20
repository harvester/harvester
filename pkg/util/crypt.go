package util

import (
	"strings"

	"github.com/tredoe/osutil/user/crypt/common"
	"github.com/tredoe/osutil/user/crypt/sha512_crypt"
)

func CompareByShadow(key, shadowLine string) bool {
	shadowSplits := strings.Split(shadowLine, ":")
	if len(shadowSplits) < 2 {
		return false
	}
	passwdHash := shadowSplits[1]
	c := sha512_crypt.New()
	return c.Verify(passwdHash, []byte(key)) == nil
}

func GetEncrptedPasswd(key string) (string, error) {
	c := sha512_crypt.New()
	salt := common.Salt{}
	saltBytes := salt.Generate(16)
	return c.Generate([]byte(key), saltBytes)
}
