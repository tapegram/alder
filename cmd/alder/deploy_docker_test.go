//go:build deploy

package main

// Built with -tags deploy (CI's deploy job), the Ssh target's tests start their box as a Docker
// container from testdata/deploy/Dockerfile when Docker answers (deploy_ssh_test.go).
func init() { dockerBox = true }
