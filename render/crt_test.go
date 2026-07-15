package render

import "testing"

func TestDefaultCRTOptions(t *testing.T) {
	o := DefaultCRTOptions()
	if o.Curvature <= 0 || o.ScanIntensity <= 0 || o.Aberration <= 0 || o.Vignette <= 0 {
		t.Fatalf("default CRT options should be positive: %+v", o)
	}
}

func TestNewCRT(t *testing.T) {
	if NewCRT() == nil {
		t.Fatal("NewCRT returned nil")
	}
}
