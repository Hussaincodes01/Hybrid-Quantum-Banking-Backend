"""FINIX Mock Bank API — standalone demo banking service.

Implements the BankAdapter contract from DATA_SOURCES_AND_MOCK_API.md as a
real HTTP service on localhost:9090. 8 seeded accounts, deterministic data.

Usage:
    pip install flask
    python mock_bank_api.py          # runs on :9090
"""

import hashlib
import secrets
import time
from datetime import datetime, timezone
from flask import Flask, jsonify, request

app = Flask(__name__)

# ── Seeded accounts (mirrors mock.go seed data) ──────────────────────────────

ACCOUNTS = [
    {"accountNumber": "12345678901", "ifsc": "SBIN0001234", "bankName": "State Bank of India", "branch": "MG Road, Bangalore", "accountType": "savings", "balancePaise": 35000000, "upiId": "jiyad@sbi", "holderName": "Jiyad"},
    {"accountNumber": "98765432109", "ifsc": "ICIC0000456", "bankName": "ICICI Bank", "branch": "Anna Salai, Chennai", "accountType": "savings", "balancePaise": 28000000, "upiId": "venkat@icici", "holderName": "Venkat"},
    {"accountNumber": "56789012345", "ifsc": "KKBK0007789", "bankName": "Kotak Mahindra Bank", "branch": "Bandra West, Mumbai", "accountType": "savings", "balancePaise": 60000000, "upiId": "shubham@oksbi", "holderName": "RD Shubham"},
    {"accountNumber": "11112222333", "ifsc": "HDFC0004321", "bankName": "HDFC Bank", "branch": "Corporate Banking", "accountType": "current", "balancePaise": 0, "upiId": "electricity@hdfcbank", "holderName": "BESCOM Electricity Board"},
    {"accountNumber": "22223333444", "ifsc": "IOCL0000123", "bankName": "Indian Oil Corporation", "branch": "LPG Division", "accountType": "current", "balancePaise": 0, "upiId": "bharatgas@indianoil", "holderName": "Bharat Gas Agency"},
    {"accountNumber": "33334444555", "ifsc": "PYTM0001234", "bankName": "Paytm Payments Bank", "branch": "Noida", "accountType": "current", "balancePaise": 0, "upiId": "amazon@paytm", "holderName": "Amazon India"},
    {"accountNumber": "44445555666", "ifsc": "HDFC0000987", "bankName": "HDFC Bank", "branch": "Brokerage Division", "accountType": "current", "balancePaise": 0, "upiId": "zerodha@hdfc", "holderName": "Zerodha Broking"},
    {"accountNumber": "55556666777", "ifsc": "PUNB0001234", "bankName": "Punjab National Bank", "branch": "Education Division", "accountType": "current", "balancePaise": 0, "upiId": "school@pnb", "holderName": "Delhi Public School"},
]

# Index by lowercased UPI ID and account number for fast lookup
_by_upi = {a["upiId"].lower(): a for a in ACCOUNTS}
_by_acc = {a["accountNumber"]: a for a in ACCOUNTS}
_by_ifsc_acc = {(a["ifsc"], a["accountNumber"]): a for a in ACCOUNTS}

# In-memory state
_connections: dict[str, dict] = {}
_txns: dict[str, list[dict]] = {}  # keyed by userId


def _now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _error(msg: str, code: str, status: int):
    return jsonify({"error": msg, "code": code}), status


# ── 1. POST /api/v1/verify/upi ──────────────────────────────────────────────

@app.post("/api/v1/verify/upi")
def verify_upi():
    body = request.get_json(silent=True) or {}
    upi_id = (body.get("upiId") or "").strip().lower()
    if not upi_id:
        return _error("upi_id is required", "VALIDATION_ERROR", 400)
    acct = _by_upi.get(upi_id)
    if acct:
        return jsonify({"status": "verified", "holderName": acct["holderName"], "verified": True})
    return jsonify({"status": "not_found", "holderName": "", "verified": False})


# ── 2. POST /api/v1/verify/ifsc ─────────────────────────────────────────────

@app.post("/api/v1/verify/ifsc")
def verify_ifsc():
    body = request.get_json(silent=True) or {}
    ifsc = (body.get("ifsc") or "").strip().upper()
    acc_num = (body.get("accountNumber") or "").strip()
    if not ifsc or not acc_num:
        return _error("ifsc and accountNumber are required", "VALIDATION_ERROR", 400)
    acct = _by_ifsc_acc.get((ifsc, acc_num))
    if acct:
        return jsonify({
            "status": "verified",
            "holderName": acct["holderName"],
            "bankName": acct["bankName"],
            "branch": acct["branch"],
            "verified": True,
        })
    return jsonify({"status": "not_found", "holderName": "", "bankName": "", "branch": "", "verified": False})


# ── 3. GET /api/v1/accounts/{accountId}/balance ─────────────────────────────

@app.get("/api/v1/accounts/<account_id>/balance")
def get_balance(account_id: str):
    account_id = account_id.strip()
    # Try account number first, then UPI ID
    acct = _by_acc.get(account_id) or _by_upi.get(account_id.lower())
    if acct:
        return jsonify({
            "accountId": account_id,
            "balancePaise": acct["balancePaise"],
            "currency": "INR",
            "asOf": _now_iso(),
        })
    return _error("account not found", "NOT_FOUND", 404)


# ── 4. POST /api/v1/connections ──────────────────────────────────────────────

@app.post("/api/v1/connections")
def connect():
    body = request.get_json(silent=True) or {}
    user_id = (body.get("userId") or "").strip()
    if not user_id:
        return _error("userId is required", "VALIDATION_ERROR", 400)

    now = _now_iso()
    conn = _connections.get(user_id)
    if not conn:
        conn = {
            "connectionId": "bankconn_" + secrets.token_hex(8),
            "connectedAt": now,
        }
        _connections[user_id] = conn

    conn.update({
        "provider": body.get("provider") or "mock-bank",
        "baseUrl": body.get("baseUrl") or "https://mock-bank.local",
        "clientId": body.get("clientId") or "FINIX-mock",
        "connected": True,
        "sandbox": body.get("sandbox", True),
        "lastHeartbeatAt": now,
    })
    return jsonify(conn)


# ── 5. GET /api/v1/connections/{connectionId} ────────────────────────────────

@app.get("/api/v1/connections/<connection_id>")
def get_connection(connection_id: str):
    for conn in _connections.values():
        if conn.get("connectionId") == connection_id:
            return jsonify(conn)
    return _error("connection not found", "NOT_FOUND", 404)


# ── 6. GET /api/v1/transactions ─────────────────────────────────────────────

@app.get("/api/v1/transactions")
def fetch_transactions():
    user_id = request.args.get("userId", "").strip()
    if not user_id:
        return _error("userId query parameter is required", "VALIDATION_ERROR", 400)

    since = request.args.get("since", "")
    limit = min(int(request.args.get("limit", 50)), 200)
    offset = max(int(request.args.get("offset", 0)), 0)

    all_events = list(_txns.get(user_id, []))

    # Seeded fallback events when user has no recorded txns
    if not all_events:
        now = time.time()
        all_events = [
            {
                "eventId": "mockevt_" + secrets.token_hex(6),
                "bankCustomerId": user_id,
                "bankAccountId": "",
                "amountPaise": 250000,
                "counterparty": "venkat@icici",
                "channel": "upi",
                "eventTimestamp": datetime.fromtimestamp(now - 86400, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "source": "mock-bank",
                "metadata": None,
            },
            {
                "eventId": "mockevt_" + secrets.token_hex(6),
                "bankCustomerId": user_id,
                "bankAccountId": "",
                "amountPaise": 120000,
                "counterparty": "electricity@hdfcbank",
                "channel": "upi",
                "eventTimestamp": datetime.fromtimestamp(now - 259200, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "source": "mock-bank",
                "metadata": None,
            },
        ]

    # Filter by since
    if since:
        all_events = [e for e in all_events if e["eventTimestamp"] >= since]

    total = len(all_events)
    page = all_events[offset: offset + limit]
    return jsonify({"events": page, "total": total, "hasMore": offset + limit < total})


# ── 7. POST /api/v1/payments ────────────────────────────────────────────────

@app.post("/api/v1/payments")
def submit_payment():
    body = request.get_json(silent=True) or {}
    user_id = (body.get("userId") or "").strip()
    from_account = (body.get("fromAccount") or "").strip()
    beneficiary = (body.get("beneficiary") or "").strip()
    amount = body.get("amountPaise", 0)
    currency = body.get("currency", "INR")
    method = body.get("method", "upi")

    if not user_id or not beneficiary or not isinstance(amount, int) or amount <= 0:
        return _error("userId, beneficiary, and positive amountPaise are required", "VALIDATION_ERROR", 400)

    # Debit payer if known
    payer_acct = _by_upi.get(from_account.lower()) or _by_acc.get(from_account)
    if payer_acct:
        if payer_acct["balancePaise"] < amount:
            return _error("insufficient balance", "INSUFFICIENT_BALANCE", 422)
        payer_acct["balancePaise"] -= amount

    ref = "MOCKPAY" + secrets.token_hex(8).upper()
    now = _now_iso()

    event = {
        "eventId": ref,
        "bankCustomerId": user_id,
        "bankAccountId": from_account,
        "amountPaise": amount,
        "counterparty": beneficiary,
        "channel": method,
        "eventTimestamp": now,
        "source": "mock-bank",
        "metadata": None,
    }
    _txns.setdefault(user_id, []).append(event)

    return jsonify({
        "reference": ref,
        "status": "processed",
        "amountPaise": amount,
        "timestamp": now,
    })


# ── Health check ─────────────────────────────────────────────────────────────

@app.get("/api/v1/health")
def health():
    return jsonify({"status": "ok", "service": "mock-bank", "accounts": len(ACCOUNTS)})


if __name__ == "__main__":
    print("FINIX Mock Bank API on http://localhost:9090")
    print(f"Seeded {len(ACCOUNTS)} accounts: {', '.join(a['upiId'] for a in ACCOUNTS[:3])}...")
    app.run(host="0.0.0.0", port=9090, debug=True)
