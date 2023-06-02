package fuzz

import (
	"errors"
	"fmt"
	"net"

	"k8s.io/apimachinery/pkg/util/sets"
)

// FreePorts tries to find the free ports and returns them or error.
func FreePorts(amount int) ([]int, error) {
	set := sets.NewInt()

	for {
		if set.Len() >= amount {
			break
		}
		// #nosec G102
		lis, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("failed to get free ports, %v", err)
		}

		addr, ok := lis.Addr().(*net.TCPAddr)
		if !ok {
			_ = lis.Close()
			return nil, errors.New("failed to get a TCP address")
		}
		set.Insert(addr.Port)
		_ = lis.Close()
	}

	return set.List(), nil
}
