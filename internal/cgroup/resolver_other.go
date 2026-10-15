//go:build !linux

package cgroup

import "errors"

func CheckRoot(string) error                { return errors.New("Linux cgroup v2 required") }
func CheckRootVersion(string, string) error { return errors.New("Linux cgroup required") }
func NativeHandle(string) ([]byte, error)   { return nil, errors.New("name_to_handle_at requires Linux") }
