package analysis

import "testing"

func TestDetectSpike(t *testing.T) {
	flat := []int64{100, 110, 90, 105, 95, 100, 108, 92}

	if v := DetectSpike(600, flat); !v.Spike || v.ZScore < 3 {
		t.Fatalf("6x baseline must spike: %+v", v)
	}
	if v := DetectSpike(115, flat); v.Spike {
		t.Fatalf("within noise must not spike: %+v", v)
	}
	if v := DetectSpike(40, []int64{0, 0, 0, 0, 0, 0}); v.Spike {
		t.Fatalf("below volume floor must not spike: %+v", v)
	}
	if v := DetectSpike(60, []int64{0, 0, 0, 0, 0, 0}); !v.Spike {
		t.Fatalf("60 events over dead-zero baseline must spike: %+v", v)
	}
	if v := DetectSpike(1000, []int64{100, 100}); v.Spike {
		t.Fatalf("insufficient history must stay quiet: %+v", v)
	}
	// constant nonzero history, big jump
	if v := DetectSpike(900, []int64{200, 200, 200, 200, 200, 200}); !v.Spike {
		t.Fatalf("4.5x flat baseline must spike: %+v", v)
	}
}
