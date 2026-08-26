//go:build !linux

package app

import "errors"

func RunDNSProxy(_, _, _ string) error { return errors.New("dns-proxy is supported only on Linux") }
