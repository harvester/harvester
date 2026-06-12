package util

import (
	"strings"

	"github.com/tredoe/osutil/user/crypt"
	"github.com/tredoe/osutil/user/crypt/apr1_crypt"
	"github.com/tredoe/osutil/user/crypt/common"
	"github.com/tredoe/osutil/user/crypt/md5_crypt"
	"github.com/tredoe/osutil/user/crypt/sha256_crypt"
	"github.com/tredoe/osutil/user/crypt/sha512_crypt"
)

func CompareByShadow(key, shadowLine string) bool {
	shadowSplits := strings.Split(shadowLine, ":")
	if len(shadowSplits) < 2 {
		return false
	}
	passwdHash := shadowSplits[1]

	passwdHashSplits := strings.Split(passwdHash, "$")
	if len(passwdHashSplits) < 4 {
		return false
	}
	var c crypt.Crypter

	prefix := passwdHashSplits[1]
	switch prefix {
	case "1":
		c = md5_crypt.New()
	case "5":
		c = sha256_crypt.New()
	case "6":
		c = sha512_crypt.New()
	case "apr1":
		c = apr1_crypt.New()
	default:
		return false
	}

	return c.Verify(passwdHash, []byte(key)) == nil
}

func GetEncryptedPasswd(key string) (string, error) {
	c := sha512_crypt.New()
	salt := common.Salt{}
	saltBytes := salt.Generate(16)
	return c.Generate([]byte(key), saltBytes)
}
