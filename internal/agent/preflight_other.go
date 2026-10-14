//go:build !linux

package agent

import "errors"

func CheckCgroupNamespace(string) error { return errors.New("Linux required") }
