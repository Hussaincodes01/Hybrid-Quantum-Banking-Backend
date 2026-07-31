package preprocess

import (
	"math"
	"testing"
)

// newTestArtifact builds a small, hand-verified artifact: 2 numeric + 1
// categorical column, StandardScaler on all three (categorical scaled after
// label-encoding, exactly as the training notebooks do).
func newTestArtifact(t *testing.T) *Artifact {
	t.Helper()
	a := &Artifact{
		Model:        "test",
		FeatureOrder: []string{"amount", "age", "kind"},
		ScaledColumns: []string{"amount", "age", "kind"},
		Scaler: Scaler{
			Type:  "standard",
			Mean:  []float64{100, 10, 1}, // kind mean=1 (codes 0,1,2 -> mean 1)
			Scale: []float64{50, 2, 1},
		},
		Categoricals: map[string][]string{
			"kind": {"a", "b", "c"}, // a=0, b=1, c=2
		},
	}
	if err := a.finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	return a
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestEncode(t *testing.T) {
	a := newTestArtifact(t)
	for val, want := range map[string]int{"a": 0, "b": 1, "c": 2} {
		got, ok := a.Encode("kind", val)
		if !ok || got != want {
			t.Errorf("Encode(kind,%q)=%d,%v want %d,true", val, got, ok, want)
		}
	}
	if _, ok := a.Encode("kind", "zzz"); ok {
		t.Error("unseen value should return ok=false")
	}
	if _, ok := a.Encode("nope", "a"); ok {
		t.Error("unknown column should return ok=false")
	}
}

func TestScaleValue(t *testing.T) {
	a := newTestArtifact(t)
	// (150-100)/50 = 1.0 ; (8-10)/2 = -1.0
	if got := a.scaleValue("amount", 150); !approx(got, 1.0) {
		t.Errorf("scale amount=150 got %v want 1.0", got)
	}
	if got := a.scaleValue("age", 8); !approx(got, -1.0) {
		t.Errorf("scale age=8 got %v want -1.0", got)
	}
	// mean input -> 0
	if got := a.scaleValue("amount", 100); !approx(got, 0) {
		t.Errorf("scale amount=mean got %v want 0", got)
	}
}

func TestBuildVectorHappyPath(t *testing.T) {
	a := newTestArtifact(t)
	vec, defaulted := a.BuildVector(
		map[string]float64{"amount": 150, "age": 12},
		map[string]string{"kind": "c"}, // code 2 -> (2-1)/1 = 1.0
	)
	if len(defaulted) != 0 {
		t.Fatalf("unexpected defaulted cols: %v", defaulted)
	}
	want := []float32{1.0, 1.0, 1.0} // (150-100)/50, (12-10)/2, (2-1)/1
	for i := range want {
		if math.Abs(float64(vec[i]-want[i])) > 1e-6 {
			t.Errorf("vec[%d]=%v want %v", i, vec[i], want[i])
		}
	}
}

func TestBuildVectorDefaultsToMean(t *testing.T) {
	a := newTestArtifact(t)
	// Omit "age" and give an unseen categorical: both should scale to 0 and be
	// reported as defaulted.
	vec, defaulted := a.BuildVector(
		map[string]float64{"amount": 100}, // amount=mean -> 0
		map[string]string{"kind": "unseen"},
	)
	if vec[0] != 0 {
		t.Errorf("amount at mean should be 0, got %v", vec[0])
	}
	if vec[1] != 0 {
		t.Errorf("missing age should default to mean (0), got %v", vec[1])
	}
	if vec[2] != 0 {
		t.Errorf("unseen categorical should default to mean (0), got %v", vec[2])
	}
	if len(defaulted) != 2 {
		t.Errorf("expected 2 defaulted cols (age, kind), got %v", defaulted)
	}
}

func TestFinalizeRejectsInconsistent(t *testing.T) {
	bad := &Artifact{
		FeatureOrder:  []string{"x"},
		ScaledColumns: []string{"x"},
		Scaler:        Scaler{Mean: []float64{1, 2}, Scale: []float64{1}},
	}
	if err := bad.finalize(); err == nil {
		t.Error("expected error for mean/scale length mismatch")
	}
}

func TestDefaultArtifactPath(t *testing.T) {
	cases := map[string]string{
		`/a/b/transaction_risk_model.onnx`: `/a/b/transaction_preprocess.json`,
		`..\..\models\MuleAccountDetection.onnx`: `..\..\models\mule_preprocess.json`,
	}
	// only checks directory joining; base is caller-supplied
	got := DefaultArtifactPath(`/a/b/transaction_risk_model.onnx`, "transaction_preprocess.json")
	if got != cases[`/a/b/transaction_risk_model.onnx`] {
		t.Errorf("got %q", got)
	}
	got = DefaultArtifactPath(`..\..\models\MuleAccountDetection.onnx`, "mule_preprocess.json")
	if got != cases[`..\..\models\MuleAccountDetection.onnx`] {
		t.Errorf("got %q", got)
	}
}
