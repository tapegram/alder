# CA bundle

`cacert.pem` is Mozilla's CA bundle as extracted by curl.

- Source: https://curl.se/ca/cacert.pem
- Fetched: 2026-10-17
- SHA-256: `a41b5d356aea97a529fe27e0f7316d2f9d946d75927476cf9cf1b90637d00505`

The deploy image is `scratch` and has no trust store; `internal/deploy/oci.go` writes this file
to `/etc/ssl/certs/ca-certificates.crt` so the app's TLS clients (Turso, HTTPS APIs) verify.

To refresh:

```
curl -fsSL https://curl.se/ca/cacert.pem -o internal/deploy/cacerts/cacert.pem
shasum -a 256 internal/deploy/cacerts/cacert.pem
```

Then update the date and hash above. A new bundle changes every image digest, as it should.
