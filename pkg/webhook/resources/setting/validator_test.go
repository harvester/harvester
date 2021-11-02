package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func Test_validateOvercommitConfig(t *testing.T) {
	tests := []struct {
		name   string
		args   *v1beta1.Setting
		errMsg string
	}{
		{
			name: "invalid json",
			args: &v1beta1.Setting{
				ObjectMeta: v1.ObjectMeta{Name: "overcommit-config"},
				Value:      `{"cpu":100,"memory":100,"storage":100`,
			},
			errMsg: `Invalid JSON: {"cpu":100,"memory":100,"storage":100`,
		},
		{
			name: "cpu undercommmit",
			args: &v1beta1.Setting{
				ObjectMeta: v1.ObjectMeta{Name: "overcommit-config"},
				Value:      `{"cpu":99,"memory":100,"storage":100}`,
			},
			errMsg: `Cannot undercommit. Should be greater than or equal to 100 but got 99`,
		},
		{
			name: "memory undercommmit",
			args: &v1beta1.Setting{
				ObjectMeta: v1.ObjectMeta{Name: "overcommit-config"},
				Value:      `{"cpu":100,"memory":98,"storage":100}`,
			},
			errMsg: `Cannot undercommit. Should be greater than or equal to 100 but got 98`,
		},
		{
			name: "storage undercommmit",
			args: &v1beta1.Setting{
				ObjectMeta: v1.ObjectMeta{Name: "overcommit-config"},
				Value:      `{"cpu":100,"memory":100,"storage":97}`,
			},
			errMsg: `Cannot undercommit. Should be greater than or equal to 100 but got 97`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOvercommitConfig(tt.args)
			if tt.errMsg != "" {
				assert.Equal(t, tt.errMsg, err.Error())
			}
		})

	}
}
