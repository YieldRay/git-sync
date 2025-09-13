package main

import (
	"log"
	"os/exec"
)

func Map[T any, R any](input []T, f func(T) R) []R {
	result := make([]R, len(input))
	for i, v := range input {
		result[i] = f(v)
	}
	return result
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	// Send child process output to the same log file
	writer := log.Writer()
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}
