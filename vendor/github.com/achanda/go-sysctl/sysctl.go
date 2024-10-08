package sysctl

import (
        "errors"
        "io/ioutil"
        "os"
        "runtime"
        "strings"
)

const (
        sysctlDir = "/proc/sys/"
)

var invalidKeyError = errors.New("could not find the given key")

func Get(name string) (string, error) {
        if runtime.GOOS != "linux" {
                os.Exit(1)
        }
        path := sysctlDir + strings.Replace(name, ".", "/", -1)
        data, err := ioutil.ReadFile(path)
        if err != nil {
                return "",invalidKeyError
        }
        return strings.TrimSpace(string(data)),nil
}

func Set(name string, value string) error {
        if runtime.GOOS != "linux" {
                os.Exit(1)
        }
        path := sysctlDir + strings.Replace(name, ".", "/", -1)
        err := ioutil.WriteFile(path, []byte(value), 0644)
        return err
}
