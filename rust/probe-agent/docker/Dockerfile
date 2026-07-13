# Probe Agent — Production Docker Image
# Multi-stage build: builder → runtime
FROM rust:1.76-slim AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential pkg-config libelf-dev libssl-dev libbpf-dev clang llvm lld cmake ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
COPY . .

# Build only the probe-agent binary (skip eBPF which needs nightly + bpf target)
RUN cargo build --release -p probe-agent

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata libelf1 libssl3 && \
    rm -rf /var/lib/apt/lists/* && \
    adduser --disabled-password --gecos "" --uid 1000 probe

COPY --from=builder /workspace/target/release/probe-agent /usr/local/bin/
COPY probe-agent/config.yaml /etc/probe-agent/config.yaml

RUN mkdir -p /var/lib/probe-agent/cache && chown -R probe:probe /var/lib/probe-agent

USER probe
EXPOSE 9091
ENTRYPOINT ["/usr/local/bin/probe-agent"]
CMD ["--config", "/etc/probe-agent/config.yaml"]
