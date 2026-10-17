package registry

import "testing"

func TestKind(t *testing.T) {
	for host, want := range map[string]string{
		"ghcr.io": "GITHUB", "docker.io": "DOCKER", "registry.gitlab.com": "GITLAB",
		"us-docker.pkg.dev":                            "GOOGLE_ARTIFACT",
		"123456789012.dkr.ecr.us-east-1.amazonaws.com": "AWS_ECR",
	} {
		if got, ok := Kind(host); !ok || got != want {
			t.Errorf("Kind(%q) = %q, %v; want %q", host, got, ok, want)
		}
	}
	for _, host := range []string{"quay.io", "pkg.dev", ".pkg.dev", "amazonaws.com", "evilghcr.io", ""} {
		if k, ok := Kind(host); ok {
			t.Errorf("Kind(%q) = %q, want none", host, k)
		}
	}
}
