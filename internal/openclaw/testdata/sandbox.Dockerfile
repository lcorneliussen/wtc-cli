# check=skip=InvalidDefaultArgInFrom
# Test sandbox: OpenClaw's default sandbox image, built as its documentation
# says (docs/gateway/sandboxing/images-and-setup.md), plus the wtc binary under
# test, taken from the gateway test image, so that an agent's sandboxed shell
# runs the same wtc as the gateway's.
ARG GATEWAY_IMAGE
FROM ${GATEWAY_IMAGE} AS gateway

FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      bash ca-certificates curl git jq python3 ripgrep \
    && rm -rf /var/lib/apt/lists/*
COPY --from=gateway /usr/local/bin/wtc /usr/local/bin/wtc
RUN useradd --create-home --shell /bin/bash sandbox
USER sandbox
WORKDIR /home/sandbox
RUN wtc --help >/dev/null
CMD ["sleep", "infinity"]
