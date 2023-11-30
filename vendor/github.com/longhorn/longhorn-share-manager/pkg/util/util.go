package util

import (
	"fmt"
	"os"

	"github.com/mitchellh/go-ps"
)

// FindProcessByName finds a process by name and returns the process
func FindProcessByName(name string) (*os.Process, error) {
	processes, err := ps.Processes()
	if err != nil {
		return nil, fmt.Errorf("failed to list processes")
	}

	for _, process := range processes {
		if process.Executable() == name {
			return os.FindProcess(process.Pid())
		}
	}

	return nil, fmt.Errorf("process %s is not found", name)
}
