package util

import (
	"fmt"
	"strconv"
)

func StrictAtoi(s string) (int, error) {
	if len(s) > 1 {
		if s[0] == '0' {
			return 0, fmt.Errorf("leading zero in input: %s", s)
		}

		if s[0] == '+' {
			return 0, fmt.Errorf("no allowed symbol in input: %s", s)
		}

		if s[0] == '-' && s[1] == '0' {
			return 0, fmt.Errorf("leading zero in input: %s", s)
		}

	}

	i, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return i, nil
}
