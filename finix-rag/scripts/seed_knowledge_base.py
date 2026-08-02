"""Seed the RAG knowledge base.

Qdrant starts empty, so every RAG query has nothing to retrieve: the query
errors and the backend silently degrades to the plain LLM. This script writes a
small FINIX/Indian-banking corpus, renders it to PDF (the ingestion pipeline is
OCR-oriented and only accepts PDF/images), and pushes it through the real
SecureIngestor so the content goes through the same PII-masking, chunking,
embedding and encrypted-storage path as any other document.

Run with the RAG dependencies importable and Qdrant reachable:

    cd finix-rag
    set PYTHONPATH=src
    python scripts/seed_knowledge_base.py
"""
from __future__ import annotations

import os
import sys
from pathlib import Path

import fitz  # PyMuPDF — already a transitive dependency of the readers

REPO = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(REPO / "src"))

# Deliberately factual, demo-relevant content. Each document is a topic the
# chatbot is likely to be asked about during a walkthrough.
DOCUMENTS: dict[str, str] = {
    "finix_platform_overview": """
FINIX Platform Overview

FINIX is a secure retail banking application for the Indian market. It combines
account aggregation, UPI payments, goal-based savings, portfolio tracking and a
fraud-detection layer in a single app.

Authentication. Customers sign in with their 10-digit Central KYC (cKYC) number
and a 6-digit PIN. Sessions are short-lived JWTs valid for 15 minutes and are
bound to the device fingerprint recorded at registration, so a stolen token
cannot be replayed from another handset. High-risk actions require step-up
authentication with a biometric and, where configured, an OTP.

Accounts. Balances are aggregated across linked banks through the Account
Aggregator framework. Net worth is computed as total assets, meaning bank
balances plus investment holdings, minus outstanding liabilities such as loans.

Payments. Transfers are made over UPI, IMPS, NEFT and RTGS. Every transfer is
scored by the risk engine before the debit is allowed to settle. A single
transaction may not exceed the per-transaction ceiling of Rs 10,00,000.

Goals. Customers create savings goals with a target amount, a target date and a
monthly contribution. Goals can be paused, resumed, contributed to, or dissolved
with the funds returned to the linked account.
""",
    "fraud_and_mule_accounts": """
Fraud Detection, Mule Accounts and the Risk Engine

A mule account is a bank account used to receive and move the proceeds of fraud.
The account holder is often an unwitting participant recruited with an offer of
easy money, or a victim whose credentials were phished. Because the funds pass
through a legitimate-looking account, mule networks are how stolen money is
laundered and made hard to trace. Allowing an account to be used this way is a
criminal offence in India, even when the holder did not commit the original
fraud.

FINIX scores every transaction with a two-stage model. First a graph neural
network scores the recipient account for mule-like behaviour, using signals such
as in-degree and out-degree, the number of unique senders and receivers, and the
ratio of incoming to outgoing value. That recipient score then feeds a
transaction risk classifier alongside the amount relative to the customer's
average, the hour of day, velocity in the last hour, device posture such as a
rooted device or an emulator, and the session trust score.

The result is a risk level of low, medium or high. A medium result requires the
customer to acknowledge a warning and step up authentication. A high result
blocks the transfer outright and starts a cooling-off period during which
further transfers are refused, so a customer under social-engineering pressure
cannot be talked into retrying immediately.

Warning signs of a scam: urgency and threats of account suspension, a request
for an OTP, PIN or CVV, a shortened link, a promise of a refund or prize, or a
bank message arriving from a personal mobile number rather than an official
sender ID. A bank will never ask for an OTP, PIN or CVV.
""",
    "indian_income_tax_regimes": """
Old and New Income Tax Regimes in India

Indian taxpayers may choose between two regimes each year.

The old regime has higher headline rates but permits a wide range of deductions
and exemptions, including Section 80C for investments such as PPF, ELSS and life
insurance premiums, Section 80D for health insurance premiums, Section 80CCD(1B)
for the additional National Pension System contribution, and House Rent
Allowance.

The new regime offers lower slab rates but removes most of those deductions. It
suits taxpayers who do not have significant deductions to claim: someone paying
little rent, holding few tax-saving investments and carrying no home loan
interest will usually pay less under it.

The correct choice depends on the individual's deduction profile, not on income
alone. A taxpayer with large 80C, 80D and HRA claims frequently pays less under
the old regime despite its higher rates, while a taxpayer with few deductions
usually pays less under the new one. FINIX compares both using the customer's
actual income and declared deductions and reports the projected liability under
each.
""",
    "upi_and_payment_safety": """
UPI Payments and Safe Practice

UPI is India's real-time retail payment system, operated by NPCI. Money moves
instantly between accounts using a Virtual Payment Address of the form
name@bank, so no account number or IFSC is exchanged.

A merchant QR code encodes a UPI deep link containing the payee address, and
optionally a payee name and a fixed amount. When the QR fixes the amount the
payer cannot change it. When it does not, the payer enters the amount. FINIX
only acts on QR codes that are valid UPI links, so scanning an unrelated code
cannot start a payment.

Safe practice. Money is only ever received without approval; a request to
"approve" or enter a PIN in order to RECEIVE money is always fraudulent, because
a UPI PIN is required only to send. Verify the payee name shown by the app
before confirming. Treat a QR code sent over chat with suspicion. Never share an
OTP, UPI PIN or the app's screen with anyone claiming to be from the bank, a
marketplace or a delivery service.

If money is sent in error or under a scam, report it through the app's fraud
reporting flow immediately and to the National Cyber Crime Reporting Portal.
Speed matters, because funds can be frozen only before they are withdrawn.
""",
    "account_security_controls": """
Account Security Controls in FINIX

Emergency freeze. A customer who suspects compromise can freeze the account from
the security screen. While frozen, outgoing transfers are refused. Unfreezing
requires biometric verification, and in production an OTP as well.

Device binding. Each session is tied to the device fingerprint captured at
registration. The fingerprint is derived from stable device characteristics and
is checked on every request, so a session lifted from one device will not
authenticate from another.

Biometric keys. When the handset supports it, FINIX generates a P-256 key pair
inside the device's secure hardware. The private key never leaves that hardware
and cannot be used without a biometric. Enrolling a new fingerprint or face
invalidates the key, so someone who adds their own biometric to a stolen phone
does not inherit access.

Cooling-off. After a blocked transaction, further transfers are refused for a
configured window. This exists specifically to defeat social-engineering, where
the attacker's leverage depends on keeping the victim acting quickly.

Monitoring. Suspicious SMS can be scanned in-app, security events are logged to
an immutable audit ledger, and every risk decision carries a plain-language
explanation of why it was made.
""",
}



COLLECTION = "finix_documents"
DENSE_VECTOR = "text-dense"   # the name llama-index's retriever queries
EMBED_DIM = 1024              # BGE-M3


def prepare_collection(qdrant_url: str, api_key: str) -> None:
    """(Re)create the collection with the named dense vector the reader expects."""
    from qdrant_client import QdrantClient
    from qdrant_client.models import Distance, VectorParams

    client = QdrantClient(url=qdrant_url, api_key=api_key)
    existing = {c.name for c in client.get_collections().collections}
    if COLLECTION in existing:
        info = client.get_collection(COLLECTION)
        vectors = info.config.params.vectors
        # A dict means named vectors; a bare VectorParams means the unnamed
        # default, which the reader cannot address.
        if isinstance(vectors, dict) and DENSE_VECTOR in vectors:
            print(f"  collection {COLLECTION} already uses '{DENSE_VECTOR}'")
            return
        print(f"  recreating {COLLECTION}: vector is unnamed, reader needs '{DENSE_VECTOR}'")
        client.delete_collection(COLLECTION)

    client.create_collection(
        collection_name=COLLECTION,
        vectors_config={DENSE_VECTOR: VectorParams(size=EMBED_DIM, distance=Distance.COSINE)},
    )
    print(f"  created {COLLECTION} with named vector '{DENSE_VECTOR}' ({EMBED_DIM}d)")


def write_pdf(name: str, body: str, out_dir: Path) -> Path:
    """Render one document to a simple text PDF."""
    out_dir.mkdir(parents=True, exist_ok=True)
    path = out_dir / f"{name}.pdf"

    doc = fitz.open()
    # Wrap manually: insert_textbox needs pre-broken lines to paginate sanely.
    margin, width, leading = 56, 483, 14
    lines: list[str] = []
    for para in body.strip().split("\n\n"):
        words, line = para.replace("\n", " ").split(), ""
        for w in words:
            trial = f"{line} {w}".strip()
            if len(trial) > 92:
                lines.append(line)
                line = w
            else:
                line = trial
        lines.append(line)
        lines.append("")

    per_page = 52
    for i in range(0, len(lines), per_page):
        page = doc.new_page()
        page.insert_textbox(
            fitz.Rect(margin, margin, margin + width, 780),
            "\n".join(lines[i:i + per_page]),
            fontsize=10.5,
            fontname="helv",
        )
    doc.save(path)
    doc.close()
    return path


def main() -> int:
    os.environ.setdefault("QDRANT_HOST", "localhost")
    os.environ.setdefault("QDRANT_PORT", "6333")
    os.environ.setdefault("QDRANT_API_KEY", "local-dev")

    out_dir = REPO / "data" / "seed_corpus"
    print(f"rendering {len(DOCUMENTS)} documents -> {out_dir}")
    paths = [write_pdf(n, b, out_dir) for n, b in DOCUMENTS.items()]

    from finix_rag.ingestion.secure_ingestor import SecureIngestor

    qdrant_url = f"http://{os.environ['QDRANT_HOST']}:{os.environ['QDRANT_PORT']}"
    print(f"connecting to Qdrant at {qdrant_url}")

    # Create the collection explicitly with the NAMED vector the read path uses.
    # Left to itself the writer auto-creates an unnamed vector, while the
    # retriever queries "text-dense", so every search failed with
    #   Wrong input: Not existing vector name error: text-dense
    # even though the points were present. Pre-creating it makes both agree.
    prepare_collection(qdrant_url, os.environ["QDRANT_API_KEY"])
    # The constructor's parameter is `api_key`, not `qdrant_api_key`.
    ingestor = SecureIngestor(
        qdrant_url=qdrant_url,
        api_key=os.environ["QDRANT_API_KEY"],
    )

    ok = 0
    for p in paths:
        try:
            res = ingestor.ingest_file(p, metadata={"source": "finix-seed"}, user_id="system")
            print(f"  {p.name}: {res.get('chunks_ingested', '?')} chunks")
            ok += 1
        except Exception as exc:  # keep going: one bad document should not abort the seed
            print(f"  {p.name}: FAILED {type(exc).__name__}: {exc}")

    print(f"\ningested {ok}/{len(paths)} documents")
    return 0 if ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
