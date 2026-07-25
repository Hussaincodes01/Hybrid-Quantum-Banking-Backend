package repo

import (
	"context"
	"fmt"
	"time"
)

type InsuranceRow struct {
	PolicyID        string
	UserID          string
	PolicyType      string
	Insurer         string
	SumAssuredPaise int64
	PremiumPaise    int64
	NextDueDate     string
}

func (r *Repo) ListInsuranceByUser(ctx context.Context, userID string) ([]InsuranceRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT policy_id, user_id, policy_type, insurer, sum_assured_paise, premium_paise, next_due_date
		 FROM insurance_policies WHERE user_id = $1`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list insurance: %w", err)
	}
	defer rows.Close()
	var policies []InsuranceRow
	for rows.Next() {
		var p InsuranceRow
		if err := rows.Scan(&p.PolicyID, &p.UserID, &p.PolicyType, &p.Insurer, &p.SumAssuredPaise, &p.PremiumPaise, &p.NextDueDate); err != nil {
			return nil, fmt.Errorf("scan insurance: %w", err)
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (r *Repo) ListAllInsurance(ctx context.Context) (map[string][]InsuranceRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT policy_id, user_id, policy_type, insurer, sum_assured_paise, premium_paise, next_due_date FROM insurance_policies`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all insurance: %w", err)
	}
	defer rows.Close()
	result := make(map[string][]InsuranceRow)
	for rows.Next() {
		var p InsuranceRow
		if err := rows.Scan(&p.PolicyID, &p.UserID, &p.PolicyType, &p.Insurer, &p.SumAssuredPaise, &p.PremiumPaise, &p.NextDueDate); err != nil {
			return nil, fmt.Errorf("scan insurance: %w", err)
		}
		result[p.UserID] = append(result[p.UserID], p)
	}
	return result, rows.Err()
}

type LoanRow struct {
	LoanID           string
	UserID           string
	Lender           string
	LoanType         string
	OutstandingPaise int64
	EMIPaise         int64
	InterestRate     float64
	RemainingMonths  int
}

func (r *Repo) ListLoansByUser(ctx context.Context, userID string) ([]LoanRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT loan_id, user_id, lender, loan_type, outstanding_paise, emi_paise, interest_rate, remaining_months
		 FROM loan_details WHERE user_id = $1`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list loans: %w", err)
	}
	defer rows.Close()
	var loans []LoanRow
	for rows.Next() {
		var l LoanRow
		if err := rows.Scan(&l.LoanID, &l.UserID, &l.Lender, &l.LoanType, &l.OutstandingPaise, &l.EMIPaise, &l.InterestRate, &l.RemainingMonths); err != nil {
			return nil, fmt.Errorf("scan loan: %w", err)
		}
		loans = append(loans, l)
	}
	return loans, rows.Err()
}

func (r *Repo) ListAllLoans(ctx context.Context) (map[string][]LoanRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT loan_id, user_id, lender, loan_type, outstanding_paise, emi_paise, interest_rate, remaining_months FROM loan_details`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all loans: %w", err)
	}
	defer rows.Close()
	result := make(map[string][]LoanRow)
	for rows.Next() {
		var l LoanRow
		if err := rows.Scan(&l.LoanID, &l.UserID, &l.Lender, &l.LoanType, &l.OutstandingPaise, &l.EMIPaise, &l.InterestRate, &l.RemainingMonths); err != nil {
			return nil, fmt.Errorf("scan loan: %w", err)
		}
		result[l.UserID] = append(result[l.UserID], l)
	}
	return result, rows.Err()
}

type NotificationItemRow struct {
	ID        string
	UserID    string
	Category  string
	Title     string
	Body      string
	IsRead    bool
	CreatedAt time.Time
}

func (r *Repo) ListNotificationItemsByUser(ctx context.Context, userID string) ([]NotificationItemRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, category, title, body, is_read, created_at
		 FROM notification_items WHERE user_id = $1 ORDER BY created_at DESC LIMIT 100`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list notification items: %w", err)
	}
	defer rows.Close()
	var items []NotificationItemRow
	for rows.Next() {
		var n NotificationItemRow
		if err := rows.Scan(&n.ID, &n.UserID, &n.Category, &n.Title, &n.Body, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notification item: %w", err)
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

// ============================================================
// Sprint 1: Risk Detection & Validation Repositories
// ============================================================

// RealtimeDetectionRow represents a real-time fraud detection record
type RealtimeDetectionRow struct {
	ID           string
	TxID         string
	UserID       string
	Detector     string
	Score        float64
	Label        string
	FeaturesJSON string // JSONB as string
	CreatedAt    time.Time
}

// SaveRealtimeDetection persists a real-time detection
func (r *Repo) SaveRealtimeDetection(ctx context.Context, d RealtimeDetectionRow) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO realtime_detections (tx_id, user_id, detector, score, label, features_jsonb, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		d.TxID, d.UserID, d.Detector, d.Score, d.Label, d.FeaturesJSON, d.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save realtime detection: %w", err)
	}
	return nil
}

// ListRealtimeDetectionsByTx retrieves detections for a transaction
func (r *Repo) ListRealtimeDetectionsByTx(ctx context.Context, txID string) ([]RealtimeDetectionRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tx_id, user_id, detector, score, label, features_jsonb, created_at
		 FROM realtime_detections WHERE tx_id = $1 ORDER BY created_at DESC`, txID,
	)
	if err != nil {
		return nil, fmt.Errorf("list realtime detections by tx: %w", err)
	}
	defer rows.Close()

	var detections []RealtimeDetectionRow
	for rows.Next() {
		var d RealtimeDetectionRow
		if err := rows.Scan(&d.ID, &d.TxID, &d.UserID, &d.Detector, &d.Score, &d.Label, &d.FeaturesJSON, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan realtime detection: %w", err)
		}
		detections = append(detections, d)
	}
	return detections, rows.Err()
}

// HealthScoreSnapshotRow represents a health score snapshot
type HealthScoreSnapshotRow struct {
	ID          string
	UserID      string
	Score300900 int
	Band        string
	PillarsJSON string // JSONB as string
	CreatedAt   time.Time
}

// SaveHealthScoreSnapshot persists a health score snapshot
func (r *Repo) SaveHealthScoreSnapshot(ctx context.Context, s HealthScoreSnapshotRow) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO health_score_snapshots (user_id, score_300_900, band, pillars_jsonb, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		s.UserID, s.Score300900, s.Band, s.PillarsJSON, s.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save health score snapshot: %w", err)
	}
	return nil
}

// ListHealthScoreSnapshotsByUser retrieves health score history for a user
func (r *Repo) ListHealthScoreSnapshotsByUser(ctx context.Context, userID string, limit int) ([]HealthScoreSnapshotRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, score_300_900, band, pillars_jsonb, created_at
		 FROM health_score_snapshots WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list health score snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []HealthScoreSnapshotRow
	for rows.Next() {
		var s HealthScoreSnapshotRow
		if err := rows.Scan(&s.ID, &s.UserID, &s.Score300900, &s.Band, &s.PillarsJSON, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan health score snapshot: %w", err)
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, rows.Err()
}

// FraudReportRow represents a fraud report (for listing)
type FraudReportRow struct {
	ID          string
	UserID      string
	TxID        *string // nullable
	Reporter    string
	Details     string
	Status      string
	CreatedAt   time.Time
}

// SaveFraudReport persists a fraud report
func (r *Repo) SaveFraudReport(ctx context.Context, fr FraudReportRow) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO fraud_reports (user_id, tx_id, reporter, details, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		fr.UserID, fr.TxID, fr.Reporter, fr.Details, fr.Status, fr.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save fraud report: %w", err)
	}
	return nil
}

// ListFraudReportsByUser retrieves fraud reports for a user
func (r *Repo) ListFraudReportsByUser(ctx context.Context, userID string) ([]FraudReportRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, tx_id, reporter, details, status, created_at
		 FROM fraud_reports WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list fraud reports: %w", err)
	}
	defer rows.Close()

	var reports []FraudReportRow
	for rows.Next() {
		var fr FraudReportRow
		var txID *string
		if err := rows.Scan(&fr.ID, &fr.UserID, &txID, &fr.Reporter, &fr.Details, &fr.Status, &fr.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan fraud report: %w", err)
		}
		fr.TxID = txID
		reports = append(reports, fr)
	}
	return reports, rows.Err()
}

// RiskValidationRow represents a risk validation from Python RAG
type RiskValidationRow struct {
	ID                  string
	TxID                string
	AgreesWithML        bool
	SuggestedLevel      string
	Confidence          float64
	Rationale           string
	RecommendedAction   string
	CitationsJSON       string
	XAIExplanation      string
	Applied             bool
	NewStatus           string
	ValidatedAt         time.Time
}

// SaveRiskValidation persists a risk validation result
func (r *Repo) SaveRiskValidation(ctx context.Context, v RiskValidationRow) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO risk_validations (
			tx_id, agrees_with_ml, suggested_level, confidence, rationale,
			recommended_action, citations_jsonb, xai_explanation, applied, new_status, validated_at
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		v.TxID, v.AgreesWithML, v.SuggestedLevel, v.Confidence, v.Rationale,
		v.RecommendedAction, v.CitationsJSON, v.XAIExplanation, v.Applied, v.NewStatus, v.ValidatedAt,
	)
	if err != nil {
		return fmt.Errorf("save risk validation: %w", err)
	}
	return nil
}

// GetRiskValidationByTx retrieves the validation for a transaction
func (r *Repo) GetRiskValidationByTx(ctx context.Context, txID string) (RiskValidationRow, bool, error) {
	var v RiskValidationRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, tx_id, agrees_with_ml, suggested_level, confidence, rationale,
		        recommended_action, citations_jsonb, xai_explanation, applied, new_status, validated_at
		 FROM risk_validations WHERE tx_id = $1`, txID,
	).Scan(&v.ID, &v.TxID, &v.AgreesWithML, &v.SuggestedLevel, &v.Confidence, &v.Rationale,
		&v.RecommendedAction, &v.CitationsJSON, &v.XAIExplanation, &v.Applied, &v.NewStatus, &v.ValidatedAt)
	if err != nil {
		return RiskValidationRow{}, false, fmt.Errorf("get risk validation: %w", err)
	}
	return v, true, nil
}
