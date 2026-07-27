# FINIX — Schema Reconciliation: Backend ↔ ML Training ↔ Canonical Docs

**Date:** 2026-07-27
**Sources analysed**
- `api-docs/FINIX Dataset Features.docx` — *"Master Relational Dataset Specification v1.0"* — the declared **single source of truth** for all FINIX AI models (8 tables: Users, Accounts, Transactions, Devices, Sessions, Location History, Historical Behaviour, External Context).
- `api-docs/FINIX_Srishti_Corrected.docx` — verified mathematics for all 15 models; §2.1 defines the Transaction-Risk **RiskSignal_vector** and §3 the compliance-checked inputs.

**Compared against (in `FINIX_PRODUCTION`)**
- **Backend schema** — `backend/internal/domain/transaction/risk_engine.go` (`RiskSignal`) + `backend/migrations/*.sql` (Postgres tables).
- **ML training schema** — `backend/ml/train_fraud_model.py` (fraud/risk model feature set) and `train_behaviour_model.py`.

---

## 1. Verdict at a glance

| Layer | Matches the canonical docs? |
|---|---|
| ML training `feature_cols` ↔ Go `RiskSignal` | ✅ **Were already consistent with each other** (both 9 features, identical order; the trainer literally mirrors the struct). |
| Those two ↔ Srishti §2.1 RiskSignal_vector (12 features) | ❌ **Lagged by 4 features** → **corrected this pass** (now 13-feature parity). |
| Backend Postgres tables ↔ Dataset's 8 relational tables | ⚠️ **Partial** — 4 of 8 entities exist; 4 are computed ephemerally, not persisted. Transactions table missing 3 columns + uses user+recipient rather than account-graph edges. |
| Mule-GNN / SMS / Health-Score training data | ❌ Dataset defines them; **no training scripts exist** for GNN or SMS (only fraud + behaviour). |

---

## 2. Model 1 — Transaction Risk Score feature vector  *(corrected this pass)*

Canonical target = Srishti §2.1.2 `RiskSignal_vector` (12 terms). Raw columns to derive each exist in the Dataset doc's tables.

| Srishti §2.1 feature | Dataset doc raw source | Go `RiskSignal` (before) | ML `feature_cols` (before) | Status |
|---|---|---|---|---|
| g(AmountVsAverage) | transaction_amount ÷ average_transaction_amount_category | `AmountVsAverage` | `amount_vs_average` | ✅ matched |
| IsNewRecipient | (derived) | `IsNewRecipient` | `is_new_recipient` | ✅ matched |
| FailedPINAttempts | sessions.failed_pin_attempts | `FailedPINAttempts` | `failed_pin_attempts` | ✅ matched |
| VelocityTerm | historical_behaviour.transactions_last_1h | `VelocityCount1Hr` | `velocity_count_1hr` | ✅ matched |
| BalanceImpact | accounts.current_balance | `BalanceImpact` | `balance_impact` | ✅ matched |
| BehaviourDrift | Model 9 (biometrics) | `BehaviourDrift` | `behaviour_drift` | ✅ matched |
| SessionTrustScore | sessions.session_trust_score | `SessionTrustScore` | `session_trust_score` | ✅ matched |
| RecipientGNNScore | Model 5 output | `RecipientGNNScore` | `recipient_gnn_score` | ✅ matched |
| **GeoVelocityFlag** | latitude/longitude + last_transaction_lat/long + previous_transaction_timestamp | **absent** | **absent** | ❌→ **ADDED** |
| **DayOfWeekZScore** | external_context.day_of_week + weekday spend history | (raw `HourOfDay` only) | (raw `hour_of_day` only) | ❌→ **ADDED** (HourOfDay retained as extra) |
| **MarketContextTerm** | external_context.market_volatility_index | **absent** | **absent** | ❌→ **ADDED** |
| **StructuringFlag** | 24h same/new-recipient counts (§2.1.1) | **absent** | **absent** | ❌→ **ADDED** |

**Correction applied:** both the Go `RiskSignal` struct + heuristic (`risk_engine.go`, `heuristic.go`) and the trainer (`ml/train_fraud_model.py`) now carry the 4 missing features, in an **identical 13-column order** (`featureCount = 13`, mirrored by `feature_cols`). The heuristic scores them with the §2.1.3 rule-based weights (Geo +20, DayOfWeek +min(10·z,10), Market +8, Structuring +25). Build + `go test ./internal/domain/transaction/` pass; `train_fraud_model.py` compiles.

**Still required to make the new features live (not done here — flagged):**
1. **Retrain + re-export the ONNX model** — `python ml/train_fraud_model.py` now emits a **13-input** `model.onnx`; the committed one is 9-input. The default (heuristic) build is unaffected; the `onnx`-tagged build must use the retrained model, and its version dir should bump (v1 → v2) with `model_version` recorded on each assessment (Assessment already carries `ModelVersion`).
2. **Populate the 4 fields from data** — the service constructors (`service.go`, `aiml.go`) currently leave them at spec-valid neutral defaults (Geo=false, DayOfWeekZScore=0 → excluded per §2.1.1, Market=false, Structuring=false), so scoring is unchanged until wired. Wiring needs: prior GPS + timestamp (GeoVelocityFlag via Haversine/900 km/h), a market-volatility feed (MarketContextTerm), weekday spend history (DayOfWeekZScore ≥ 4 obs), and the 24h recipient-count query (StructuringFlag).

---

## 3. Master Relational Dataset (8 tables) ↔ Backend Postgres

Backend tables present: `users, accounts, transactions, sessions, goals, audit_events, beneficiary_links, blockchain_events, challenges, chatbot_history, consent_grants, fraud_reports, freeze_states, health_score_snapshots, notification_settings, realtime_detections, risk_validations`.

| Dataset entity | Backend table | Status / gap |
|---|---|---|
| Users | `users` | ⚠️ present, but `age, occupation, monthly_income, home_country` are not first-class columns (profile/derived). |
| Accounts | `accounts` | ⚠️ present; verify `account_type, account_age_days, current_balance` columns exist. |
| Transactions | `transactions` | ⚠️ see §3.1 — 3 columns missing; account-graph edges absent. |
| Devices | — | ❌ **no table** (device signals handled ephemerally / SIM-binding). Needed for Model 1 + Model 9. |
| Sessions | `sessions` | ⚠️ present; verify it carries `session_trust_score, otp_verified, biometric_verified, failed_pin_attempts, sim_binding_verified`. |
| Location History | — | ❌ **no table** — blocks GeoVelocityFlag persistence. |
| Historical Behaviour | — | ❌ **no table** — `transactions_last_1h/24h`, `average_transaction_amount_category`, prior lat/long computed on the fly, not stored. |
| External Context | — | ❌ **no table** — `market_volatility_index`, `hour_of_day`, `day_of_week` not persisted. |

### 3.1 `transactions` column reconciliation
Backend: `id, user_id, amount_paise, recipient, channel, status, risk_level, risk_score, xai_reason, idempotency_key, cooling_off_until, created_at, model_version`.

| Dataset column | Backend | Note |
|---|---|---|
| transaction_id | `id` (UUID) | ✅ name diff only |
| transaction_amount | `amount_paise` (int64 paise) | ⚠️ **unit diff** — dataset uses a float rupee amount; backend stores paise. Training/join must convert. |
| transaction_timestamp | `created_at` | ✅ |
| payment_channel | `channel` | ✅ name diff |
| transaction_status | `status` | ✅ name diff |
| sender_account_id / receiver_account_id | `user_id` + `recipient` (string) | ❌ **structural** — backend is user+recipient-string; dataset is account-to-account. The **Mule GNN edge list** (Accounts=nodes, Transactions=edges) cannot be derived cleanly without account-level sender/receiver. |
| transaction_type | — | ❌ missing (P2P/P2M) |
| merchant_category | — | ❌ missing (needed by Health-Score & nudging) |
| currency | — | ❌ missing |

---

## 4. Other models — dataset defined, implementation gaps

- **Model 5 (Mule GNN):** dataset derives it from Accounts(nodes)+Transactions(edges); backend has an in-memory structural-score `FraudGraph` (§2.5.1 path) but **no `train_mule_model.py`** and no account-edge persistence (§3.1). `RecipientGNNScore` is produced by the in-memory path only.
- **Model 10 (SMS Fraud):** spec §2.10 defines 5 deterministic layers + an ML layer; backend has the keyword heuristic; **no `train_bert_sms.py`** and no SMS corpus schema.
- **Model 2 (Financial Health Score):** `health_score_snapshots` table exists (good); the 7-pillar inputs (`merchant_category`, BNPL classification, portfolio overlap matrix) partly depend on the missing Transactions columns and External Context.

---

## 5. Changes applied this pass (in `FINIX_PRODUCTION`)

| File | Change |
|---|---|
| `backend/internal/domain/transaction/risk_engine.go` | `RiskSignal` +4 spec fields (GeoVelocityFlag, DayOfWeekZScore, MarketContextTerm, StructuringFlag). |
| `backend/internal/domain/transaction/heuristic.go` | `featureCount=13`; `Predict` reads all 13; `assess` scores the 4 new terms (§2.1.3); `signalToFeatures` emits 13 in canonical order. |
| `backend/ml/train_fraud_model.py` | Generates + scores the 4 new features; dataframe column order matches `signalToFeatures` indices 9–12. |
| `SCHEMA_RECONCILIATION.md` | This document. |

Verified: `go test ./internal/domain/transaction/` ✅ ; `python -m py_compile ml/train_fraud_model.py` ✅. (Full-module `go build` not re-run here — the change is confined to the transaction package + service constructors compile against the superset struct.)

## 6. Remaining corrections (prioritised, not done here)

1. **Retrain ONNX** (13-input) + bump model version dir; record `model_version` per assessment.
2. **Wire the 4 new RiskSignal fields** from real inputs (geo history, market feed, weekday history, 24h counts).
3. **Add missing Transactions columns**: `transaction_type`, `merchant_category`, `currency`; decide on account-level sender/receiver for the GNN edge list.
4. **Persist the 4 missing dataset entities** (Devices, Location History, Historical Behaviour, External Context) if the models are to train on the "single source of truth" relational dataset rather than ephemeral computation.
5. **Add `train_mule_model.py` (GCN) and `train_bert_sms.py`** to match Models 5 and 10; align their feature schemas to the dataset doc.
6. Confirm `users`/`accounts`/`sessions` carry the dataset's declared columns (age/occupation/income/home_country; account_type/age/balance; trust/otp/biometric/sim_binding).
