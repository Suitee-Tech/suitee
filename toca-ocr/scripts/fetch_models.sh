#!/bin/sh
# Baixa os modelos do Docling usados pelo /ocr/parse.
#
# O Hugging Face responde 429 (Too Many Requests) para downloads anônimos
# repetidos — e builds locais repetidos disparam isso com facilidade. Antes,
# um único 429 derrubava o build inteiro no meio da camada de modelos. Agora
# cada passo tenta de novo com espera crescente (limite de 5 min por espera).
#
# Ajustes por env (útil em CI lento ou rede instável):
#   MODEL_DOWNLOAD_ATTEMPTS=8
#   MODEL_DOWNLOAD_BASE_DELAY_SECONDS=20
#   SKIP_MODEL_DOWNLOAD=1  → não baixa nada (imagem "magra"); use só quando o
#   volume docling_models já está populado, ex.: dev local com o HF retornando
#   429. O runtime passa a depender do volume.
set -eu

if [ "${SKIP_MODEL_DOWNLOAD:-0}" = "1" ]; then
  echo "AVISO: SKIP_MODEL_DOWNLOAD=1 — modelos do Docling NÃO foram embutidos na imagem." >&2
  echo "AVISO: este build só funciona em runtime se o volume docling_models já estiver populado." >&2
  exit 0
fi

attempts="${MODEL_DOWNLOAD_ATTEMPTS:-8}"
base_delay="${MODEL_DOWNLOAD_BASE_DELAY_SECONDS:-20}"

case "$attempts" in
  ''|*[!0-9]*) echo "MODEL_DOWNLOAD_ATTEMPTS inválido: $attempts" >&2; exit 2 ;;
esac
case "$base_delay" in
  ''|*[!0-9]*) echo "MODEL_DOWNLOAD_BASE_DELAY_SECONDS inválido: $base_delay" >&2; exit 2 ;;
esac

retry() {
  label="$1"
  shift

  attempt=1
  while ! "$@"; do
    if [ "$attempt" -ge "$attempts" ]; then
      echo "ERRO: '$label' falhou após $attempt tentativa(s)." >&2
      return 1
    fi
    delay=$((base_delay * attempt))
    [ "$delay" -le 300 ] || delay=300
    echo "AVISO: '$label' falhou (tentativa $attempt/$attempts); nova tentativa em ${delay}s." >&2
    sleep "$delay"
    attempt=$((attempt + 1))
  done
}

# 1) Conjunto padrão do docling (layout, tableformer, OCR artifacts...).
retry "docling download_models" \
  python -c "from docling.utils.model_downloader import download_models; download_models()"

# 2) Artefatos que o download_models() não garante por si só.
retry "layout tableformer" \
  docling-tools models download layout tableformer -o /root/.cache/docling
retry "docling-layout-heron" \
  docling-tools models download-hf-repo docling-project/docling-layout-heron -o /root/.cache/docling

# 3) Atalhos esperados pelo docling para o tableformer.
ln -sfn /root/.cache/docling/models/docling-project--docling-models/model_artifacts/tableformer/accurate /root/.cache/docling/accurate
ln -sfn /root/.cache/docling/models/docling-project--docling-models/model_artifacts/tableformer/fast /root/.cache/docling/fast

# 4) RapidOCR (torch + onnxruntime) — sem ele o /ocr/parse falha em runtime.
retry "rapidocr" \
  docling-tools models download rapidocr -o /root/.cache/docling
