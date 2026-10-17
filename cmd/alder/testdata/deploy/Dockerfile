# The box the Ssh target's tests deploy to under `go test -tags deploy` (deploy_ssh_test.go,
# deploy_docker_test.go): Debian with sshd, curl and tar, a user `deploy` whose key the test
# copies in after the container starts, and /srv owned by it. No systemd runs in a container,
# so the target runs each build as a background process with a pid file; no Caddy either: the
# tests declare `proxy: None`.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends openssh-server curl ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir -p /run/sshd \
 && useradd -m -s /bin/sh deploy \
 && mkdir -p /srv && chown deploy /srv
EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]
