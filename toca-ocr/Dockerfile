# P1-16: base pinada por digest (resolvida em linux/amd64, 2026-10-03).
FROM python:3.12-slim@sha256:6b1f85a08c199d29d5b6d71ab9c27bd5b3b393492e01216a15758ff69c4be8b8

ENV PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DEFAULT_TIMEOUT=300 \
    PIP_RETRIES=10 \
    HF_HUB_DOWNLOAD_TIMEOUT=300 \
    DOCLING_ARTIFACTS_PATH=/root/.cache/docling \
    EASYOCR_MODULE_PATH=/root/.cache/easyocr

WORKDIR /app

# Dependências de sistema usadas por docling/opencv (decode de imagem/PDF).
RUN apt-get update && apt-get install -y --no-install-recommends \
        libgl1 \
        libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --timeout 300 --retries 10 -r requirements.txt

# Pré-baixa os modelos do docling no build, evitando cold-start/rede em runtime.
# O download_models() default não garante o layout-heron nem o RapidOCR;
# sem eles o /ocr/parse falha em runtime com "RapidOCR artifacts not found".
# Não restringir o backend/língua: o docling 2.135 baixa RapidOcr/PP-OCRv6
# (torch + onnxruntime) com `rapidocr` puro; a flag antiga deixava o diretório
# RapidOcr ausente e o OCR quebrava em produção.
#
# O script faz retry com backoff: o Hugging Face devolve 429 para downloads
# anônimos repetidos e um único 429 derrubava o build inteiro.
#
# SKIP_MODEL_DOWNLOAD=1 gera uma imagem sem os modelos (só para dev local com
# o volume docling_models já populado) — ver scripts/fetch_models.sh.
ARG SKIP_MODEL_DOWNLOAD=0
COPY scripts/fetch_models.sh /usr/local/bin/fetch_models.sh
RUN chmod +x /usr/local/bin/fetch_models.sh \
    && SKIP_MODEL_DOWNLOAD="${SKIP_MODEL_DOWNLOAD}" /usr/local/bin/fetch_models.sh

COPY app ./app

EXPOSE 5001

# Healthcheck via httpx (já é dependência) — evita exigir curl na imagem slim.
HEALTHCHECK --interval=15s --timeout=10s --retries=5 --start-period=120s \
    CMD python -c "import httpx,sys; sys.exit(0 if httpx.get('http://127.0.0.1:5001/ready', timeout=5).status_code==200 else 1)"

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "5001"]
