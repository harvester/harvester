//go:build !gojq_debug
// +build !gojq_debug

package gojq

type codeinfo struct{}

func (*compiler) appendCodeInfo(any) {}

func (*compiler) deleteCodeInfo(string) {}

func (*env) debugCodes() {}

func (*env) debugState(int, bool) {}

func (*env) debugForks(int, string) {}
