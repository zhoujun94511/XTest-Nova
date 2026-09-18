//go:build !linux && !android

package main

import "fmt"

func startDaemon(_ []string) error {
	return fmt.Errorf("daemon mode is only supported on Android/Linux")
}

func configureDaemonLog() (func(), error) { return func() {}, nil }
