"""PaddleOCR engine for financial document processing.

Handles OCR + table detection + layout analysis for scanned PDFs, images,
bank statements, and regulatory documents (RBI, SEBI).
"""
import logging
import tempfile
from pathlib import Path
from typing import Optional
from dataclasses import dataclass

logger = logging.getLogger(__name__)

# Lazy import to avoid heavy startup
_paddle_ocr = None
_paddle_structure = None


def _get_ocr():
    global _paddle_ocr
    if _paddle_ocr is None:
        from paddleocr import PaddleOCR
        _paddle_ocr = PaddleOCR(
            use_angle_cls=True,
            lang="en",
            use_gpu=False,
            show_log=False,
        )
    return _paddle_ocr


def _get_structure():
    global _paddle_structure
    if _paddle_structure is None:
        from paddleocr import PPStructure
        _paddle_structure = PPStructure(
            table=True,
            ocr=True,
            lang="en",
            use_gpu=False,
            show_log=False,
        )
    return _paddle_structure


@dataclass
class OCRResult:
    """Result from OCR processing."""
    text: str
    tables: list[dict]
    layout_elements: list[dict]
    confidence: float
    page_count: int


class PaddleOCREngine:
    """OCR engine for financial documents using PaddleOCR PP-Structure.

    Extracts text, tables, and layout information from:
    - Scanned bank statements
    - PDF regulatory documents (RBI, SEBI)
    - Financial reports and invoices
    - Handwritten annotations on forms
    """

    def __init__(self, use_gpu: bool = False):
        self.use_gpu = use_gpu

    def process_document(self, file_path: str | Path) -> OCRResult:
        """Process a document through PaddleOCR pipeline.

        Args:
            file_path: Path to PDF or image file.

        Returns:
            OCRResult with extracted text, tables, and layout.
        """
        file_path = Path(file_path)
        logger.info("Processing document: %s", file_path.name)

        if file_path.suffix.lower() == ".pdf":
            return self._process_pdf(file_path)
        elif file_path.suffix.lower() in (".png", ".jpg", ".jpeg", ".tiff", ".bmp"):
            return self._process_image(file_path)
        else:
            raise ValueError(f"Unsupported file type: {file_path.suffix}")

    def _process_pdf(self, pdf_path: Path) -> OCRResult:
        """Convert PDF pages to images then run OCR."""
        import fitz  # PyMuPDF

        doc = fitz.open(str(pdf_path))
        all_text = []
        all_tables = []
        all_layout = []
        confidences = []

        structure_engine = _get_structure()

        for page_num in range(len(doc)):
            page = doc.load_page(page_num)
            # Render page to image at 300 DPI for OCR quality
            pix = page.get_pixmap(dpi=300)
            img_path = tempfile.NamedTemporaryFile(suffix=".png", delete=False)
            pix.save(img_path.name)

            # Run PP-Structure for layout + table detection
            result = structure_engine(img_path.name)

            page_text = []
            page_tables = []
            page_layout = []

            for element in result:
                if element["type"] == "text":
                    page_text.append(element["res"]["text"])
                    confidences.append(element["res"]["confidence"])
                elif element["type"] == "table":
                    page_tables.append({
                        "html": element["res"]["html"],
                        "bbox": element["res"].get("bbox", []),
                        "confidence": element["res"].get("confidence", 0),
                    })
                page_layout.append({
                    "type": element["type"],
                    "bbox": element["res"].get("bbox", []),
                })

            all_text.extend(page_text)
            all_tables.extend(page_tables)
            all_layout.extend(page_layout)

            # Cleanup temp file
            Path(img_path.name).unlink(missing_ok=True)

        doc.close()

        avg_conf = sum(confidences) / len(confidences) if confidences else 0
        logger.info(
            "OCR complete: %d pages, %d tables, confidence=%.2f",
            len(doc), len(all_tables), avg_conf,
        )

        return OCRResult(
            text="\n".join(all_text),
            tables=all_tables,
            layout_elements=all_layout,
            confidence=avg_conf,
            page_count=len(doc),
        )

    def _process_image(self, img_path: Path) -> OCRResult:
        """Run OCR on a single image."""
        structure_engine = _get_structure()
        result = structure_engine(str(img_path))

        text_parts = []
        tables = []
        layout = []
        confidences = []

        for element in result:
            if element["type"] == "text":
                text_parts.append(element["res"]["text"])
                confidences.append(element["res"].get("confidence", 0))
            elif element["type"] == "table":
                tables.append({
                    "html": element["res"]["html"],
                    "bbox": element["res"].get("bbox", []),
                })
            layout.append({
                "type": element["type"],
                "bbox": element["res"].get("bbox", []),
            })

        avg_conf = sum(confidences) / len(confidences) if confidences else 0

        return OCRResult(
            text="\n".join(text_parts),
            tables=tables,
            layout_elements=layout,
            confidence=avg_conf,
            page_count=1,
        )

    def extract_table_data(self, html_table: str) -> list[dict]:
        """Parse HTML table to structured row data."""
        import re

        rows = []
        # Simple HTML table parser
        tr_pattern = re.compile(r"<tr[^>]*>(.*?)</tr>", re.DOTALL | re.IGNORECASE)
        td_pattern = re.compile(
            r"<t[dh][^>]*>(.*?)</t[dh]>", re.DOTALL | re.IGNORECASE
        )

        for tr_match in tr_pattern.finditer(html_table):
            cells = []
            for td_match in td_pattern.finditer(tr_match.group(1)):
                cell_text = re.sub(r"<[^>]+>", "", td_match.group(1)).strip()
                cells.append(cell_text)
            if cells:
                rows.append(cells)

        return rows
