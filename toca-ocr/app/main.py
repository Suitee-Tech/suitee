"""toca-ocr — wrapper FastAPI fino em volta da lib `docling`.

Converte PDFs/imagens em markdown/texto e expõe a confiança do OCR (por página e
agregada). É o único serviço Python do monorepo: isola a dependência de ML.

Contrato consumido pelo `toca-ai` (ver toca-ai/internal/services/ocr_client.go):

    POST /ocr/parse
      - multipart/form-data com campo `file`            (teste/upload direto)
      - application/json  {"source_url": "<presigned>"}  (fluxo assíncrono — primário)
    -> 200 {
         "markdown": "...",
         "text": "...",
         "pages": [{"page": 1, "ocr_score": 0.97, "layout_score": ..., "overall_score": ...}],
         "ocr_confidence": 0.94 | null,
         "provider": "docling"
       }

Autenticação: header `X-Internal-Key` validado contra a env `INTERNAL_API_KEY`
(mesmo contrato dos serviços Go — pkg/auth/middleware.go RequireInternalKey).
"""

from __future__ import annotations

import asyncio
import io
import logging
import os
from contextlib import asynccontextmanager
from typing import Any, Optional
from urllib.parse import urlparse

import httpx
from fastapi import FastAPI, Header, HTTPException, Request
from fastapi.responses import JSONResponse, Response

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("toca-ocr")

# Singleton do conversor: o carregamento dos modelos acontece uma única vez.
_converter: Any = None
_converter_error: Optional[str] = None


def get_converter() -> Any:
    global _converter
    if _converter is None:
        from docling.document_converter import DocumentConverter

        _converter = DocumentConverter()
    return _converter


@asynccontextmanager
async def lifespan(_: FastAPI):
    # Aquece os modelos em background para a primeira requisição não pagar o
    # custo de carregamento. Não bloqueia o /health.
    async def _warm() -> None:
        global _converter_error
        try:
            await asyncio.to_thread(get_converter)
            _converter_error = None
            logger.info("docling converter aquecido")
        except Exception as exc:  # noqa: BLE001
            _converter_error = type(exc).__name__
            logger.exception("falha ao aquecer o conversor docling")

    task = asyncio.create_task(_warm())
    try:
        yield
    finally:
        task.cancel()


app = FastAPI(title="toca-ocr", version="1.0.0", lifespan=lifespan)


# --------------------------------------------------------------------------- #
# Auth interna
# --------------------------------------------------------------------------- #
def _check_internal_key(provided: Optional[str]) -> None:
    expected = os.getenv("INTERNAL_API_KEY", "")
    if expected == "":
        # Dev: sem chave configurada, libera mas avisa. Em produção a env é setada.
        logger.warning("INTERNAL_API_KEY ausente; liberando requisição (modo dev)")
        return
    if provided != expected:
        raise HTTPException(status_code=401, detail="X-Internal-Key inválida ou ausente")


# --------------------------------------------------------------------------- #
# Helpers
# --------------------------------------------------------------------------- #
def _filename_from_url(url: str) -> str:
    name = os.path.basename(urlparse(url).path)
    return name or "document"


async def _download(url: str) -> bytes:
    async with httpx.AsyncClient(timeout=60.0, follow_redirects=True) as client:
        resp = await client.get(url)
        resp.raise_for_status()
        return resp.content


def _num(v: Any) -> Optional[float]:
    """Converte para float tolerando None/NaN (docling usa NaN p/ score N/A)."""
    try:
        f = float(v)
    except (TypeError, ValueError):
        return None
    if f != f:  # NaN
        return None
    return f


def _extract_confidence(result: Any) -> tuple[Optional[float], list[dict]]:
    """Lê o ConfidenceReport do docling de forma tolerante a versão."""
    pages_out: list[dict] = []
    ocr_scores: list[float] = []

    conf = getattr(result, "confidence", None)
    if conf is None:
        return None, pages_out

    pages = getattr(conf, "pages", None) or {}
    try:
        items = list(pages.items())
    except AttributeError:
        items = []

    for page_no, grade in items:
        ocr = _num(getattr(grade, "ocr_score", None))
        overall = _num(getattr(grade, "mean_grade", None))
        if overall is None:
            overall = _num(getattr(grade, "overall_score", None))
        pages_out.append(
            {
                "page": page_no,
                "ocr_score": ocr,
                "layout_score": _num(getattr(grade, "layout_score", None)),
                "parse_score": _num(getattr(grade, "parse_score", None)),
                "table_score": _num(getattr(grade, "table_score", None)),
                "overall_score": overall,
            }
        )
        if ocr is not None:
            ocr_scores.append(ocr)

    if ocr_scores:
        overall_ocr: Optional[float] = sum(ocr_scores) / len(ocr_scores)
    else:
        # Fallback: nota média do relatório (texto nativo -> OCR não roda -> None).
        overall_ocr = _num(getattr(conf, "mean_grade", None))

    return overall_ocr, pages_out


def _convert_pdf_pages(content: bytes, filename: str) -> dict:
    """Renderiza cada página do PDF e roda o Docling na imagem.

    PDFs "digitais" (ex.: RG Digital de SP) podem ter só o texto institucional
    na camada nativa; os dados biográficos ficam em imagem/vetor na página. O
    Docling, ao receber o PDF, aproveita a camada de texto e não OCRiza a
    imagem. Rasterizar a página e converter a imagem força o OCR.
    """
    import pypdfium2 as pdfium
    from docling.datamodel.base_models import DocumentStream

    pdf = pdfium.PdfDocument(content)
    converter = get_converter()
    markdowns: list[str] = []
    texts: list[str] = []
    pages_out: list[dict] = []
    scores: list[float] = []

    for index in range(len(pdf)):
        page = pdf[index]
        image = page.render(scale=2.0).to_pil()
        image_buf = io.BytesIO()
        image.save(image_buf, format="PNG")
        image_buf.seek(0)

        result = converter.convert(
            DocumentStream(name=f"{filename}.page-{index + 1}.png", stream=image_buf)
        )
        doc = result.document

        page_markdown = ""
        try:
            page_markdown = doc.export_to_markdown()
        except Exception:  # noqa: BLE001
            logger.exception("export_to_markdown falhou (pdf raster page %s)", index + 1)

        page_text = ""
        export_text = getattr(doc, "export_to_text", None)
        if callable(export_text):
            try:
                page_text = export_text()
            except Exception:  # noqa: BLE001
                page_text = ""
        if not page_text:
            page_text = page_markdown

        markdowns.append(f"<!-- page {index + 1} -->\n{page_markdown}")
        texts.append(page_text)

        page_confidence, page_items = _extract_confidence(result)
        if page_confidence is not None:
            scores.append(page_confidence)
        for item in page_items:
            pages_out.append({**item, "page": index + 1})

    overall = sum(scores) / len(scores) if scores else None
    return {
        "markdown": "\n\n".join(markdowns),
        "text": "\n\n".join(texts),
        "pages": pages_out,
        "ocr_confidence": overall,
        "provider": "docling-raster",
    }


def _convert_bytes(content: bytes, filename: str) -> dict:
    """Síncrono e CPU-bound — chamar via asyncio.to_thread."""
    from docling.datamodel.base_models import DocumentStream

    if content[:5] == b"%PDF-" or filename.lower().endswith(".pdf"):
        return _convert_pdf_pages(content, filename)

    converter = get_converter()
    stream = DocumentStream(name=filename, stream=io.BytesIO(content))
    result = converter.convert(stream)
    doc = result.document

    markdown = ""
    try:
        markdown = doc.export_to_markdown()
    except Exception:  # noqa: BLE001
        logger.exception("export_to_markdown falhou")

    text = ""
    export_text = getattr(doc, "export_to_text", None)
    if callable(export_text):
        try:
            text = export_text()
        except Exception:  # noqa: BLE001
            text = ""
    if not text:
        text = markdown

    ocr_confidence, pages = _extract_confidence(result)

    return {
        "markdown": markdown,
        "text": text,
        "pages": pages,
        "ocr_confidence": ocr_confidence,
        "provider": "docling",
    }


# --------------------------------------------------------------------------- #
# Rotas
# --------------------------------------------------------------------------- #
@app.get("/health")
@app.get("/live")
@app.get("/health/live")
def health_live() -> dict:
    return {"status": "ok"}


@app.get("/ready")
@app.get("/health/ready")
def health_ready() -> Response:
    if _converter_error is not None:
        return JSONResponse(
            status_code=503,
            content={"status": "not_ready", "reason": "model_load_failed"},
        )
    if _converter is None:
        return JSONResponse(
            status_code=503,
            content={"status": "warming_models"},
        )
    return JSONResponse(status_code=200, content={"status": "ok"})


@app.get("/metrics")
def metrics() -> Response:
    ready = 1 if _converter is not None and _converter_error is None else 0
    payload = (
        "# HELP toca_ocr_ready Whether the Docling converter is ready.\n"
        "# TYPE toca_ocr_ready gauge\n"
        f"toca_ocr_ready {ready}\n"
    )
    return Response(
        content=payload,
        media_type="text/plain; version=0.0.4; charset=utf-8",
    )


@app.post("/ocr/parse")
async def parse(
    request: Request,
    x_internal_key: Optional[str] = Header(default=None),
):
    _check_internal_key(x_internal_key)

    ctype = request.headers.get("content-type", "")
    if ctype.startswith("multipart/form-data"):
        form = await request.form()
        upload = form.get("file")
        if upload is None or not hasattr(upload, "read"):
            raise HTTPException(status_code=400, detail="campo 'file' ausente no form")
        content = await upload.read()
        filename = getattr(upload, "filename", None) or "document"
    elif ctype.startswith("application/json"):
        body = await request.json()
        source_url = (body or {}).get("source_url")
        if not source_url:
            raise HTTPException(status_code=400, detail="'source_url' ausente no corpo JSON")
        filename = (body or {}).get("filename") or _filename_from_url(source_url)
        try:
            content = await _download(source_url)
        except httpx.HTTPError as exc:
            raise HTTPException(status_code=502, detail=f"falha ao baixar source_url: {exc}") from exc
    else:
        raise HTTPException(status_code=415, detail=f"content-type não suportado: {ctype}")

    if not content:
        raise HTTPException(status_code=400, detail="documento vazio")

    try:
        out = await asyncio.to_thread(_convert_bytes, content, filename)
    except Exception as exc:  # noqa: BLE001
        logger.exception("falha na conversão OCR")
        raise HTTPException(status_code=502, detail=f"falha no OCR: {exc}") from exc

    return JSONResponse(out)
