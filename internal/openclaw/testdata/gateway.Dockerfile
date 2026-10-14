# check=skip=InvalidDefaultArgInFrom
# Test gateway: the pinned OpenClaw image plus what a gateway needs before it
# can publish a session's worktree, and the wtc binary under test.
#
# OpenClaw 2026.10.2 verifies Git attributes with `git var GIT_ATTR_GLOBAL`,
# which needs Git 2.42; the image ships 2.39. Publication also needs `gh` on
# PATH, which the image does not ship either. A gateway that runs its agents'
# commands in sandbox containers starts them with the Docker CLI, through a
# mounted socket.
ARG OPENCLAW_IMAGE

FROM debian:bookworm-slim AS tools
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates curl xz-utils build-essential libssl-dev libcurl4-gnutls-dev libexpat1-dev zlib1g-dev && \
    rm -rf /var/lib/apt/lists/*

ARG GIT_VERSION=2.47.3
ARG GIT_SHA256=9c2eb1250781b3e5bfef098572d07fdf132d67e6c065e4307332ade9819a1501
RUN set -eu; \
    curl --fail --silent --show-error --location --output /tmp/git.tar.xz \
      "https://mirrors.edge.kernel.org/pub/software/scm/git/git-${GIT_VERSION}.tar.xz"; \
    echo "${GIT_SHA256}  /tmp/git.tar.xz" | sha256sum --check --strict; \
    tar -xf /tmp/git.tar.xz -C /tmp; \
    make -C "/tmp/git-${GIT_VERSION}" -j"$(nproc)" prefix=/usr/local DESTDIR=/out \
      NO_TCLTK=1 NO_GETTEXT=1 NO_PERL=1 NO_PYTHON=1 install >/tmp/git-build.log 2>&1 || { tail -20 /tmp/git-build.log; exit 1; }

ARG TARGETARCH
ARG GH_VERSION=2.101.0
ARG GH_SHA256_AMD64=9bca2d1c16825f109907a23307628a2f0698fbf99662b73a5cf0b020293072b8
ARG GH_SHA256_ARM64=b57e8063f18862647c9d22727c32e9da1b963f8bf9db648fe123a6975695640f
RUN set -eu; \
    case "${TARGETARCH:-amd64}" in \
      amd64) sum="$GH_SHA256_AMD64" ;; \
      arm64) sum="$GH_SHA256_ARM64" ;; \
      *) echo "unsupported arch: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl --fail --silent --show-error --location --output /tmp/gh.tar.gz \
      "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${TARGETARCH}.tar.gz"; \
    echo "${sum}  /tmp/gh.tar.gz" | sha256sum --check --strict; \
    tar -xzf /tmp/gh.tar.gz -C /tmp; \
    install -D -m 0755 "/tmp/gh_${GH_VERSION}_linux_${TARGETARCH}/bin/gh" /out/usr/local/bin/gh

# Docker publishes no checksums; these were taken from the pinned tarballs.
ARG DOCKER_VERSION=28.5.1
ARG DOCKER_SHA256_AMD64=5c0d19f31fece1accd0358bb8cff591fe25d7b6cba19f0fd412cbfdc07f75ff6
ARG DOCKER_SHA256_ARM64=de54e37157f45a43f42f6271302372d95c0eb992cc35ecaee74989bb14058c94
RUN set -eu; \
    case "${TARGETARCH:-amd64}" in \
      amd64) arch=x86_64; sum="$DOCKER_SHA256_AMD64" ;; \
      arm64) arch=aarch64; sum="$DOCKER_SHA256_ARM64" ;; \
      *) echo "unsupported arch: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl --fail --silent --show-error --location --output /tmp/docker.tgz \
      "https://download.docker.com/linux/static/stable/${arch}/docker-${DOCKER_VERSION}.tgz"; \
    echo "${sum}  /tmp/docker.tgz" | sha256sum --check --strict; \
    tar -xzf /tmp/docker.tgz -C /tmp docker/docker; \
    install -D -m 0755 /tmp/docker/docker /out/usr/local/bin/docker

FROM ${OPENCLAW_IMAGE}
USER root
COPY --from=tools /out/usr/local/ /usr/local/
COPY wtc /usr/local/bin/wtc
USER node
RUN git --version && gh --version && docker --version && wtc --help >/dev/null
