package client

import (
	"fmt"
	"strings"
)

type TaskError struct {
	ReplicaErrors []ReplicaError
}

type ReplicaError struct {
	Address string
	Message string
}

func (e TaskError) Error() string {
	var errs []string
	for _, re := range e.ReplicaErrors {
		errs = append(errs, re.Error())
	}

	if errs == nil {
		return "Unknown"
	}

	return strings.Join(errs, "; ")
}

func (e ReplicaError) Error() string {
	return fmt.Sprintf("%v: %v", e.Address, e.Message)
}
