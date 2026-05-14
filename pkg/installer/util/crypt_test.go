package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareByShadow(t *testing.T) {
	testCases := []struct {
		Name     string
		password string
		shadow   string
		output   bool
	}{
		{
			Name:     "match",
			password: "1",
			shadow:   "rancher:$6$MxtBRHZbMLzf2vrg$tLa71bzOYDLZTMzQT1wWtYQAK3wS0mqiaOkppyOUWwo8AgsBqgVvo5b2wsgkrbtYhZlJXAK9bzPudRAxOZn1H1:18578:0:99999:7:::",
			output:   true,
		},
		{
			Name:     "mismatch",
			password: "123",
			shadow:   "rancher:$6$MxtBRHZbMLzf2vrg$tLa71bzOYDLZTMzQT1wWtYQAK3wS0mqiaOkppyOUWwo8AgsBqgVvo5b2wsgkrbtYhZlJXAK9bzPudRAxOZn1H1:18578:0:99999:7:::",
			output:   false,
		},
		{
			Name:     "invalid",
			password: "123",
			shadow:   "rancher:$6",
			output:   false,
		},
		{
			Name:     "match md5",
			password: "123456",
			shadow:   "rancher:$1$u9auViW8$PZ3di3aAQHHUtKS3jvgP3/:18578:0:99999:7:::",
			output:   true,
		},
		{
			Name:     "mismatch md5",
			password: "1234",
			shadow:   "rancher:$1$u9auViW8$PZ3di3aAQHHUtKS3jvgP3/:18578:0:99999:7:::",
			output:   false,
		},
		{
			Name:     "match sha256",
			password: "123abc",
			shadow:   "rancher:$5$6LAKtLP0k6eSLylm$iPmjvy9x2KFJdqNTMotoQ84OZunCFrKcctw.1l0Ho27:18578:0:99999:7:::",
			output:   true,
		},
		{
			Name:     "mismatch sha256",
			password: "abc123",
			shadow:   "rancher:$5$6LAKtLP0k6eSLylm$iPmjvy9x2KFJdqNTMotoQ84OZunCFrKcctw.1l0Ho27:18578:0:99999:7:::",
			output:   false,
		},
	}
	for _, tCase := range testCases {
		actual := CompareByShadow(tCase.password, tCase.shadow)
		assert.Equal(t, tCase.output, actual)
	}
}

func TestF(t *testing.T) {
	s, e := GetEncryptedPasswd("1")
	t.Log(s)
	t.Log(e)
}
