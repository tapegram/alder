// Package cacerts carries the Mozilla CA bundle the deploy image ships at
// /etc/ssl/certs/ca-certificates.crt, where Go's crypto/x509 looks on Linux. See README.md.
package cacerts

import _ "embed"

// PEM is the vendored bundle.
//
//go:embed cacert.pem
var PEM []byte
