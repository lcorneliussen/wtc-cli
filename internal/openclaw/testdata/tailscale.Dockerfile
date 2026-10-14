# Test gateway image plus Tailscale, for tests that sign people in through a
# tailnet. Each container runs its own tailscaled with its own state, so the
# test never touches the Tailscale of the machine that runs it.
ARG BASE_IMAGE

FROM debian:bookworm-slim AS tools
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates curl && rm -rf /var/lib/apt/lists/*
ARG TARGETARCH
ARG TAILSCALE_VERSION=1.102.3
ARG TAILSCALE_SHA256_AMD64=36ddd9b51be57ffc2990cf76323cfa13643bfbb1b8a969f6183fa164741cdef5
ARG TAILSCALE_SHA256_ARM64=a0fa1b154af8c61f862a2259f559f7396d96c0225f4a863eae2333e1546bbe25
RUN set -eu; \
    case "${TARGETARCH:-amd64}" in \
      amd64) sum="$TAILSCALE_SHA256_AMD64" ;; \
      arm64) sum="$TAILSCALE_SHA256_ARM64" ;; \
      *) echo "unsupported arch: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl --fail --silent --show-error --location --output /tmp/ts.tgz \
      "https://pkgs.tailscale.com/stable/tailscale_${TAILSCALE_VERSION}_${TARGETARCH}.tgz"; \
    echo "${sum}  /tmp/ts.tgz" | sha256sum --check --strict; \
    tar -xzf /tmp/ts.tgz -C /tmp; \
    install -D -m 0755 "/tmp/tailscale_${TAILSCALE_VERSION}_${TARGETARCH}/tailscale" /out/tailscale; \
    install -D -m 0755 "/tmp/tailscale_${TAILSCALE_VERSION}_${TARGETARCH}/tailscaled" /out/tailscaled

FROM ${BASE_IMAGE}
USER root
COPY --from=tools /out/tailscale /out/tailscaled /usr/local/bin/
USER node
RUN tailscale version >/dev/null
