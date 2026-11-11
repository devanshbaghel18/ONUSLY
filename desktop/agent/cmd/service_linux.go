//go:build !windows

package main

import "errors"

func runServiceCommand(args []string) error {
	return errors.New("service management commands are only available on Windows")
}

