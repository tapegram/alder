// Package registry names the container registries `alder deploy` can push an image to. It is a
// leaf package because both the checker (E0030) and the Render target read the same table, and
// the checker cannot import internal/ir (ir imports the checker).
package registry

import "strings"

// Kind is the Render registry credential type of host: GITHUB, DOCKER, GITLAB, GOOGLE_ARTIFACT or
// AWS_ECR; ok is false for a host that is none of the five.
func Kind(host string) (kind string, ok bool) {
	switch {
	case host == "ghcr.io":
		return "GITHUB", true
	case host == "docker.io":
		return "DOCKER", true
	case host == "registry.gitlab.com":
		return "GITLAB", true
	case strings.HasSuffix(host, ".pkg.dev") && len(host) > len(".pkg.dev"):
		return "GOOGLE_ARTIFACT", true
	case strings.HasSuffix(host, ".amazonaws.com") && len(host) > len(".amazonaws.com"):
		return "AWS_ECR", true
	}
	return "", false
}
