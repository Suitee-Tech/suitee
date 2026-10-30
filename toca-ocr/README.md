# toca-ocr

Wrapper FastAPI fino em volta da biblioteca [`docling`](https://github.com/docling-project/docling).
Converte PDFs/imagens em markdown/texto e expõe a confiança do OCR. É o **único serviço Python** do
monorepo — isola a dependência de ML (torch/easyocr) do restante (Go).

Quem chama: `toca-ai` (`internal/services/ocr_client.go`), dentro da `toca-network`, sem porta pública.

## API

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/live` (`/health`) | Liveness do processo. |
| `GET` | `/ready` | Readiness; retorna `503` enquanto os modelos aquecem ou se o carregamento falhar. |
| `GET` | `/metrics` | Métricas Prometheus, incluindo `toca_ocr_ready`. |
| `POST` | `/ocr/parse` | `multipart/form-data` (campo `file`) **ou** `application/json` `{"source_url": "..."}`. |

Resposta de `/ocr/parse`:

```json
{
  "markdown": "...",
  "text": "...",
  "pages": [{ "page": 1, "ocr_score": 0.97, "layout_score": 0.99, "overall_score": 0.98 }],
  "ocr_confidence": 0.94,
  "provider": "docling"
}
```

`ocr_confidence` é a média dos `ocr_score` por página; fica `null` quando o documento é texto nativo
(o OCR não roda) ou a versão do docling não reporta confiança.

## Auth

Header `X-Internal-Key` validado contra a env `INTERNAL_API_KEY` (mesmo contrato de
`pkg/auth/middleware.go` `RequireInternalKey`). Sem a env, libera (modo dev) com aviso no log.

## Dev local

```bash
pip install -r requirements.txt
uvicorn app.main:app --host 0.0.0.0 --port 5001
curl -F file=@rg.jpg http://localhost:5001/ocr/parse
```

> A primeira conversão baixa os modelos (docling + easyocr). No Docker eles são pré-baixados no build
> e persistidos no volume `docling_models` montado em `/root/.cache`.
>
> O download no build fica em `scripts/fetch_models.sh` e repete cada passo com
> espera crescente: o Hugging Face responde `429` para downloads anônimos
> repetidos e, sem retry, um único 429 derrubava o build. Para buildar sem baixar
> nada (só quando o volume já está populado): `SKIP_MODEL_DOWNLOAD=1 docker
> compose build toca-ocr`.
