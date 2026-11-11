# =============================================================================
#  atlantis — self-healing image  (PCI / PSRE-2847)
#
#  Heals its own OS + python CVEs on EVERY rebuild instead of carrying a
#  hand-picked "apk upgrade <pkg>" list that only covers the CVEs known when it
#  was last edited. Rebuild via the atlantis pipeline (buildspec_docker.yml)
#  behind the Wiz scan-gate, redeploy, and the OS layer comes back patched with
#  no Dockerfile edit.
#
#  SCOPE NOTE — the compiled-in Go layer does NOT self-heal here. The `atlantis`
#  binary is built by CodeBuild (buildspec.yml -> `make build-service`) with the
#  pipeline's Go toolchain (aws/codebuild/amazonlinux2-x86_64-standard:4.0) and
#  vendored deps (Gopkg.lock / dep). Go-stdlib + Go-module CVEs are remediated by
#  bumping that CodeBuild image (in com.sixt.tools.pci.terraform) and/or
#  re-vendoring — tracked separately; a blind Go bump on this 2018-era `dep` fork
#  risks the build. This file heals everything the image layers on top of it.
# =============================================================================

# Curated base (its OS layer is already patched fleet-wide by the AI-patch
# pipeline). ARG so a version bump is a one-liner / --build-arg override.
ARG BASE_IMAGE=639159760825.dkr.ecr.eu-west-1.amazonaws.com/python:alpine3.23-amd64
FROM ${BASE_IMAGE}
LABEL authors="Anubhav Mishra, Luke Kysow"
LABEL maintainer="anubhav.mishra@hootsuite.com,luke.kysow@hootsuite.com"

# create atlantis user
RUN addgroup atlantis && \
    adduser -S -G atlantis atlantis

ENV ATLANTIS_HOME_DIR=/home/atlantis

# install atlantis dependencies. dumb-init and gosu come from the Alpine repos (signed packages,
# patched with the OS layer) instead of GitHub release downloads, which timed out on 8-9 Nov 2026.
# /bin/dumb-init symlink keeps the docker-entrypoint.sh shebang working.
RUN apk add --no-cache ca-certificates curl git unzip bash openssh libcap py3-boto3 dumb-init gosu && \
    [ ! -e /usr/bin/python ] && ln -s /usr/bin/python3 /usr/bin/python || true && \
    ln -sf /usr/bin/dumb-init /bin/dumb-init && \
    rm -rf /var/cache/apk/*

# ── OS self-heal: upgrade EVERY apk package each build (no stale hand-list) ───
# Replaces "apk upgrade curl libcurl" + "apk upgrade python3 py3-pip py3-boto3".
# With no package list, apk upgrades ALL installed packages to the branch-latest
# (libssl3/libcrypto3/libcurl/curl/python3/py3-pip/py3-boto3/git/musl/expat/…),
# so a newly-disclosed CVE in any installed package heals on the next rebuild.
RUN apk update && apk upgrade --no-cache && rm -rf /var/cache/apk/*

# install terraform binaries
ENV DEFAULT_TERRAFORM_VERSION=1.14.5

RUN AVAILABLE_TERRAFORM_VERSIONS="1.7.4 1.14.5" && \
    for VERSION in ${AVAILABLE_TERRAFORM_VERSIONS}; do curl -LOk https://releases.hashicorp.com/terraform/${VERSION}/terraform_${VERSION}_linux_amd64.zip && \
    mkdir -p /usr/local/bin/tf/versions/${VERSION} && \
    unzip terraform_${VERSION}_linux_amd64.zip -d /usr/local/bin/tf/versions/${VERSION} && \
    ln -s /usr/local/bin/tf/versions/${VERSION}/terraform /usr/local/bin/terraform${VERSION};rm terraform_${VERSION}_linux_amd64.zip;done && \
    ln -s /usr/local/bin/tf/versions/${DEFAULT_TERRAFORM_VERSION}/terraform /usr/local/bin/terraform

# Verify both versions installed
RUN terraform1.7.4 version && terraform1.14.5 version && terraform version

# python self-heal: pull the latest boto3 each build (the py3-boto3 apk package
# is also advanced by the blanket "apk upgrade" above)
RUN pip3 install --upgrade boto3
# copy binary
COPY atlantis /usr/local/bin/atlantis

# copy docker entrypoint
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

# restart-if-dead safety net — atlantis serves /healthz on :4141 (dumb-init is
# already PID1 via the entrypoint script's shebang, so signals are handled)
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -fsS http://localhost:4141/healthz || exit 1

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["server"]
