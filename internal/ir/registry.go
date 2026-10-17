package ir

import "alder/internal/registry"

// RegistryKind is the Render registry credential type of an image host (`GITHUB`, `DOCKER`,
// `GITLAB`, `GOOGLE_ARTIFACT`, `AWS_ECR`); ok is false for any other host. The table itself is in
// internal/registry, which the checker can import.
func RegistryKind(host string) (string, bool) { return registry.Kind(host) }
