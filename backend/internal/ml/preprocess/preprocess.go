// Package preprocess applies the exact training-time feature preprocessing that
// the two ONNX models (transaction_risk_model, MuleAccountDetection) were fit
// on, so Go serve-time inputs match Python train-time inputs byte-for-byte.
//
// The training notebooks (backend/ML training scripts/) LabelEncode every
// categorical column to an integer and then StandardScale ALL columns — numeric
// and (now-integer) categorical alike. A tree ensemble's split thresholds and a
// GNN's weights therefore live in scaled space; feeding raw values silently
// produces wrong scores. This package reproduces that transform from the
// artifact emitted by EXPORT_transaction_preprocess.py / EXPORT_mule_preprocess.py.
package preprocess

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Scaler holds StandardScaler statistics: transformed = (x - Mean) / Scale,
// applied per column in the order of Artifact.ScaledColumns.
type Scaler struct {
	Type  string    `json:"type"`
	Mean  []float64 `json:"mean"`
	Scale []float64 `json:"scale"`
}

// Artifact is the deserialized *_preprocess.json contract. Categoricals maps a
// column name to its LabelEncoder classes, where the encoded integer code equals
// the index of the string in the slice (sklearn stores classes_ in code order).
type Artifact struct {
	Model         string              `json:"model"`
	FeatureOrder  []string            `json:"feature_order"`
	ScaledColumns []string            `json:"scaled_columns"`
	Scaler        Scaler              `json:"scaler"`
	Categoricals  map[string][]string `json:"categoricals"`

	// Present only on the transaction artifact; 0 on the mule artifact.
	DecisionThresholdFraud float64 `json:"decision_threshold_fraud"`

	// scaleByCol is a derived lookup (column -> mean/scale index), built by
	// finalize so BuildVector need not scan ScaledColumns per feature.
	scaleByCol map[string]int
	catIndex   map[string]map[string]int
}

// Load reads and validates a *_preprocess.json artifact from disk.
func Load(path string) (*Artifact, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("preprocess: read %s: %w", path, err)
	}
	var a Artifact
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("preprocess: parse %s: %w", path, err)
	}
	if err := a.finalize(); err != nil {
		return nil, fmt.Errorf("preprocess: %s: %w", path, err)
	}
	return &a, nil
}

// finalize validates internal consistency and builds fast lookups. It is safe to
// call on a hand-constructed Artifact (used by tests).
func (a *Artifact) finalize() error {
	if len(a.FeatureOrder) == 0 {
		return fmt.Errorf("empty feature_order")
	}
	if len(a.Scaler.Mean) != len(a.Scaler.Scale) {
		return fmt.Errorf("scaler mean (%d) and scale (%d) length mismatch",
			len(a.Scaler.Mean), len(a.Scaler.Scale))
	}
	if len(a.ScaledColumns) != len(a.Scaler.Mean) {
		return fmt.Errorf("scaled_columns (%d) and scaler stats (%d) length mismatch",
			len(a.ScaledColumns), len(a.Scaler.Mean))
	}
	a.scaleByCol = make(map[string]int, len(a.ScaledColumns))
	for i, col := range a.ScaledColumns {
		a.scaleByCol[col] = i
	}
	a.catIndex = make(map[string]map[string]int, len(a.Categoricals))
	for col, classes := range a.Categoricals {
		m := make(map[string]int, len(classes))
		for i, c := range classes {
			m[c] = i
		}
		a.catIndex[col] = m
	}
	return nil
}

// FeatureCount is the width of the model input vector (32 for txn, 20 for mule).
func (a *Artifact) FeatureCount() int { return len(a.FeatureOrder) }

// Encode returns the LabelEncoder integer code for a categorical value. ok is
// false for an unseen value (sklearn would raise at transform time); callers
// decide the fallback (typically treat as the training mean, i.e. scaled 0).
func (a *Artifact) Encode(col, value string) (code int, ok bool) {
	m, exists := a.catIndex[col]
	if !exists {
		return 0, false
	}
	code, ok = m[value]
	return code, ok
}

// scaleValue standardizes a raw value for a named column. Columns absent from
// the scaler (should not happen for these two models) pass through unscaled.
func (a *Artifact) scaleValue(col string, raw float64) float64 {
	i, ok := a.scaleByCol[col]
	if !ok {
		return raw
	}
	s := a.Scaler.Scale[i]
	if s == 0 {
		// Zero-variance column: scikit-learn sets scale_ to 1.0, but guard anyway.
		return raw - a.Scaler.Mean[i]
	}
	return (raw - a.Scaler.Mean[i]) / s
}

// BuildVector assembles the final model input in FeatureOrder, applying label
// encoding to categoricals and StandardScaler to every column. numeric supplies
// non-categorical columns; categorical supplies the raw string values. A missing
// input, or an unseen categorical value, is treated as the training mean (scaled
// to 0) — the neutral value for a standardized feature — and its column name is
// returned in defaulted so callers can log/observe partial-input inference.
func (a *Artifact) BuildVector(numeric map[string]float64, categorical map[string]string) (vec []float32, defaulted []string) {
	vec = make([]float32, len(a.FeatureOrder))
	for i, col := range a.FeatureOrder {
		if classes, isCat := a.Categoricals[col]; isCat {
			_ = classes
			raw, ok := categorical[col]
			if !ok {
				defaulted = append(defaulted, col)
				vec[i] = float32(a.scaleValue(col, meanFor(a, col)))
				continue
			}
			code, seen := a.Encode(col, raw)
			if !seen {
				defaulted = append(defaulted, col)
				vec[i] = float32(a.scaleValue(col, meanFor(a, col)))
				continue
			}
			vec[i] = float32(a.scaleValue(col, float64(code)))
			continue
		}
		raw, ok := numeric[col]
		if !ok {
			defaulted = append(defaulted, col)
			vec[i] = float32(a.scaleValue(col, meanFor(a, col)))
			continue
		}
		vec[i] = float32(a.scaleValue(col, raw))
	}
	return vec, defaulted
}

// meanFor returns the training mean of a column (so scaleValue yields 0), or 0
// if the column has no scaler entry.
func meanFor(a *Artifact, col string) float64 {
	if i, ok := a.scaleByCol[col]; ok {
		return a.Scaler.Mean[i]
	}
	return 0
}

// DefaultArtifactPath derives the preprocess artifact path from a model path,
// e.g. ".../transaction_risk_model.onnx" -> ".../transaction_preprocess.json".
// It is a convenience for wiring; callers may override.
func DefaultArtifactPath(modelPath, artifactBase string) string {
	dir := modelPath
	if idx := strings.LastIndexAny(modelPath, `/\`); idx >= 0 {
		dir = modelPath[:idx+1]
	} else {
		dir = ""
	}
	return dir + artifactBase
}
