"""Native document text extraction (no OCR dependency).

For PDFs that already carry a text layer (the common case for reports, RBI/SEBI
circulars, generated statements) we read the text directly with PyMuPDF — this is
fast, exact, and avoids the heavy PaddleOCR stack entirely. OCR is only needed for
*scanned* pages / images, so it is used as a fallback and imported lazily.

Returns the same OCRResult shape the rest of the pipeline expects, so it is a drop-in
replacement for PaddleOCREngine.process_document().
"""
import logging
from pathlib import Path

from finix_rag.ingestion.ocr_engine import OCRResult, PaddleOCREngine

logger = logging.getLogger(__name__)

# If a PDF yields fewer than this many characters of real text per page on average,
# we assume the pages are scanned images and fall back to OCR.
_MIN_CHARS_PER_PAGE = 20

_TEXT_SUFFIXES = {".txt", ".md", ".csv"}
_IMAGE_SUFFIXES = {".png", ".jpg", ".jpeg", ".tiff", ".bmp"}


def _extract_pdf_native(pdf_path: Path) -> OCRResult | None:
    """Extract a PDF's embedded text layer with PyMuPDF.

    Returns None when the PDF has no usable text layer (i.e. it is scanned),
    signalling the caller to fall back to OCR.
    """
    import fitz  # PyMuPDF

    doc = fitz.open(str(pdf_path))
    page_texts = []
    try:
        for page in doc:
            # Block-aware extraction preserves paragraph structure: each text block
            # becomes a paragraph separated by a blank line, so `_semantic_chunk` can
            # split on paragraph boundaries instead of emitting one giant chunk.
            blocks = page.get_text("blocks")
            text_blocks = [b for b in blocks if len(b) >= 7 and b[6] == 0 and b[4].strip()]
            text_blocks.sort(key=lambda b: (round(b[1]), round(b[0])))  # top-to-bottom, left-to-right
            page_texts.append("\n\n".join(b[4].strip() for b in text_blocks))
        page_count = doc.page_count
    finally:
        doc.close()

    text = "\n\n".join(page_texts).strip()
    if page_count and len(text) < _MIN_CHARS_PER_PAGE * page_count:
        logger.info(
            "PDF '%s' has little/no text layer (%d chars over %d pages) — using OCR",
            pdf_path.name, len(text), page_count,
        )
        return None

    logger.info(
        "Extracted %d chars from '%s' via native PDF text layer (no OCR)",
        len(text), pdf_path.name,
    )
    return OCRResult(
        text=text,
        tables=[],
        layout_elements=[],
        confidence=1.0,  # a real text layer is exact, not an OCR estimate
        page_count=page_count,
    )


def extract_document(file_path: str | Path, ocr_engine: PaddleOCREngine | None = None) -> OCRResult:
    """Extract text from a document, preferring native extraction over OCR.

    - .txt/.md/.csv  -> read directly
    - .pdf           -> native text layer, OCR fallback for scanned PDFs
    - images         -> OCR (PaddleOCR) — requires requirements-ocr.txt

    Args:
        file_path: Path to the document.
        ocr_engine: Optional shared PaddleOCREngine for the OCR fallback.

    Returns:
        OCRResult with extracted text (tables only populated on the OCR path).
    """
    file_path = Path(file_path)
    suffix = file_path.suffix.lower()

    # Plain text files — trivially exact.
    if suffix in _TEXT_SUFFIXES:
        text = file_path.read_text(encoding="utf-8", errors="replace").strip()
        logger.info("Read %d chars from plain-text file '%s'", len(text), file_path.name)
        return OCRResult(text=text, tables=[], layout_elements=[], confidence=1.0, page_count=1)

    # PDFs — native first, OCR only if scanned.
    if suffix == ".pdf":
        native = _extract_pdf_native(file_path)
        if native is not None:
            return native
        return _ocr(file_path, ocr_engine)

    # Images — OCR is the only option.
    if suffix in _IMAGE_SUFFIXES:
        return _ocr(file_path, ocr_engine)

    raise ValueError(f"Unsupported file type: {suffix}")


def _ocr(file_path: Path, ocr_engine: PaddleOCREngine | None) -> OCRResult:
    """Run the PaddleOCR fallback, with a clear error if OCR extras are missing."""
    engine = ocr_engine or PaddleOCREngine()
    try:
        return engine.process_document(file_path)
    except ImportError as e:
        raise RuntimeError(
            f"'{file_path.name}' needs OCR (scanned/image) but PaddleOCR is not installed. "
            "Install it with: pip install -r requirements-ocr.txt (see its header for the "
            "huggingface-hub caveat)."
        ) from e
