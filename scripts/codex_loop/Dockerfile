ARG BASE_IMAGE=python:3.11-slim
FROM ${BASE_IMAGE}

LABEL org.opencontainers.image.title="traffic-analysis codex-loop"
LABEL org.opencontainers.image.description="Control-plane runtime for Codex Loop queue service and remote workers"

ENV PYTHONUNBUFFERED=1
ENV CODEX_LOOP_REPO_ROOT=/app
ENV CODEX_LOOP_SCRIPT_ROOT=/app/scripts/codex_loop
ENV CODEX_LOOP_RUNS_ROOT=/workspace/doc/02_acceptance/runs

WORKDIR /app

ARG INSTALL_OS_PACKAGES=0
RUN if [ "${INSTALL_OS_PACKAGES}" = "1" ]; then \
      apt-get update \
      && apt-get install -y --no-install-recommends ca-certificates git \
      && rm -rf /var/lib/apt/lists/*; \
    fi \
    && if ! getent group codexloop >/dev/null; then groupadd --gid 1000 codexloop; fi \
    && if ! getent passwd codexloop >/dev/null; then useradd --uid 1000 --gid 1000 --home-dir /home/codexloop --create-home codexloop; fi \
    && mkdir -p /app/scripts/codex_loop /workspace/doc/02_acceptance/runs \
    && chown -R codexloop:codexloop /app /workspace

COPY --chown=codexloop:codexloop . /app/scripts/codex_loop

USER 1000:1000

ENTRYPOINT ["python", "-B"]
CMD ["/app/scripts/codex_loop/queue_service.py", "serve", "--host", "0.0.0.0", "--port", "8765", "--queue-backend", "sqlite", "--queue-path", "/workspace/doc/02_acceptance/runs/.loop/queue.sqlite3", "--auth-token-env", "CODEX_LOOP_QUEUE_TOKEN", "--quiet"]
