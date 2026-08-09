# Probe Agent — Production Docker Image
# Multi-stage build: builder → runtime
FROM rust:1.93.0-slim-bookworm AS builder

ENV RUSTUP_TOOLCHAIN=1.93.0-x86_64-unknown-linux-gnu
ARG CARGO_REGISTRIES_CRATES_IO_INDEX=sparse+https://rsproxy.cn/index/
ARG CARGO_HTTP_MULTIPLEXING=false
ARG RUSTFLAGS=-C target-cpu=x86-64-v2
ENV CARGO_REGISTRIES_CRATES_IO_INDEX=${CARGO_REGISTRIES_CRATES_IO_INDEX} \
    CARGO_HTTP_MULTIPLEXING=${CARGO_HTTP_MULTIPLEXING} \
    RUSTFLAGS=${RUSTFLAGS}

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential pkg-config libelf-dev libssl-dev libbpf-dev ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
COPY . .

# Build only the probe-agent binary (skip eBPF which needs nightly + bpf target)
RUN cargo build --locked --release -p probe-agent

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata libelf1 libssl3 && \
    rm -rf /var/lib/apt/lists/* && \
    adduser --disabled-password --gecos "" --uid 1000 probe

COPY --from=builder /workspace/target/release/probe-agent /usr/local/bin/
COPY probe-agent/config.container.yaml /etc/probe-agent/config.yaml

RUN mkdir -p /var/lib/probe-agent/cache && chown -R probe:probe /var/lib/probe-agent

USER probe
EXPOSE 9091
ENTRYPOINT ["/usr/local/bin/probe-agent"]
CMD ["/etc/probe-agent/config.yaml"]
