// Package analysis holds Korugan's detectors. Rule-based analyzers are
// pure functions over aggregates: free to run, trivial to test, and they
// work with zero LLM keys configured.
package analysis

import (
	"fmt"
	"math"
)

// SpikeVerdict is the outcome of a spike check on a count series.
type SpikeVerdict struct {
	Spike    bool
	ZScore   float64
	Baseline float64 // mean of history
	Current  int64
	Detail   string
}

// minSpikeVolume guards against screaming about 3 events at 03:00.
const minSpikeVolume = 50

// DetectSpike compares the current bucket against a history of same-sized
// buckets using a z-score with a volume floor. History must not include
// the current bucket.
func DetectSpike(current int64, history []int64) SpikeVerdict {
	if len(history) < 6 {
		return SpikeVerdict{Detail: "insufficient history"}
	}
	var sum float64
	for _, h := range history {
		sum += float64(h)
	}
	mean := sum / float64(len(history))
	var variance float64
	for _, h := range history {
		d := float64(h) - mean
		variance += d * d
	}
	variance /= float64(len(history))
	sd := math.Sqrt(variance)

	v := SpikeVerdict{Baseline: mean, Current: current}
	if current < minSpikeVolume {
		v.Detail = fmt.Sprintf("below volume floor (%d < %d)", current, minSpikeVolume)
		return v
	}
	if sd == 0 {
		// Flat history: any meaningful multiple of the mean is a spike.
		if mean == 0 || float64(current) > 4*mean {
			v.Spike = true
			v.ZScore = math.Inf(1)
			v.Detail = fmt.Sprintf("flat baseline %.1f, current %d", mean, current)
		}
		return v
	}
	v.ZScore = (float64(current) - mean) / sd
	if v.ZScore >= 3 {
		v.Spike = true
		v.Detail = fmt.Sprintf("z=%.1f (current %d vs baseline %.1f±%.1f)", v.ZScore, current, mean, sd)
	}
	return v
}
