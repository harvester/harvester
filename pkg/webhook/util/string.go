package util

import (
	"fmt"
	"strconv"
)

func StrictAtoi(s string) (int, error) {
	if len(s) > 1 && s[0] == '0' {
		return 0, fmt.Errorf("leading zero in input: %s", s)
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if i < 0 {
		return 0, fmt.Errorf("negative number %d", i)
	}
	return i, nil
}
