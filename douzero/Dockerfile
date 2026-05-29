FROM ghcr.io/astral-sh/uv:python3.14-slim-trixie

WORKDIR /app

COPY pyproject.toml uv.lock* ./
RUN uv sync --frozen --no-dev

COPY src/ ./src/
COPY server.py .

ARG HF_BASE=https://huggingface.co/palemoky/douzero-baselines/resolve/main/models_onnx/douzero_WP
RUN apt-get update && apt-get install -y --no-install-recommends wget && \
    mkdir -p models && \
    for pos in landlord landlord_down landlord_up; do \
      wget -q "${HF_BASE}/${pos}.onnx" -O "models/${pos}.onnx"; \
    done && \
    apt-get purge -y wget && apt-get autoremove -y && rm -rf /var/lib/apt/lists/*

ENV DOUZERO_MODELS=/app/models
ENV PORT=2021

EXPOSE 2021

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:2021/health')"

CMD ["/app/.venv/bin/python", "server.py"]
