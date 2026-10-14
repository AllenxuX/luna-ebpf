//go:build !linux

package cgroup

import "errors"

func CheckRoot(string) error              { return errors.New("Linux cgroup v2 required") }
func NativeHandle(string) ([]byte, error) { return nil, errors.New("name_to_handle_at requires Linux") }
