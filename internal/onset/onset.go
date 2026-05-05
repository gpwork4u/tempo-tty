// Package onset detects rhythmic onsets via spectral flux.
package onset

import (
	"math"
	"sort"

	"gonum.org/v1/gonum/dsp/fourier"
)

const (
	FrameSize = 2048
	HopSize   = 512
)

func hann(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
	}
	return w
}

// Detect runs spectral flux onset detection on mono samples.
// Returns parallel slices: onset times (sec) and spectral centroids (Hz) at each onset.
func Detect(samples []float64, sr int, minGap float64) (times, centroids []float64) {
	if len(samples) < FrameSize*2 {
		return nil, nil
	}
	win := hann(FrameSize)
	fft := fourier.NewFFT(FrameSize)

	nFrames := (len(samples)-FrameSize)/HopSize + 1
	half := FrameSize/2 + 1
	mags := make([][]float64, nFrames)
	cents := make([]float64, nFrames)
	frame := make([]float64, FrameSize)
	coef := make([]complex128, half)

	for fi := 0; fi < nFrames; fi++ {
		off := fi * HopSize
		for i := 0; i < FrameSize; i++ {
			frame[i] = samples[off+i] * win[i]
		}
		fft.Coefficients(coef, frame)
		m := make([]float64, half)
		var sumMag, sumW float64
		for k, c := range coef {
			mag := math.Hypot(real(c), imag(c))
			m[k] = mag
			freq := float64(k) * float64(sr) / float64(FrameSize)
			sumMag += mag
			sumW += mag * freq
		}
		mags[fi] = m
		if sumMag > 0 {
			cents[fi] = sumW / sumMag
		}
	}

	flux := make([]float64, nFrames)
	for fi := 1; fi < nFrames; fi++ {
		var s float64
		prev, curr := mags[fi-1], mags[fi]
		for k := 0; k < half; k++ {
			d := curr[k] - prev[k]
			if d > 0 {
				s += d
			}
		}
		flux[fi] = s
	}

	hopT := float64(HopSize) / float64(sr)
	minGapFrames := int(minGap / hopT)
	if minGapFrames < 1 {
		minGapFrames = 1
	}

	const winLen = 20
	const threshK = 1.5

	lastPick := -minGapFrames
	for fi := 1; fi < nFrames-1; fi++ {
		if flux[fi] <= flux[fi-1] || flux[fi] <= flux[fi+1] {
			continue
		}
		lo, hi := fi-winLen, fi+winLen
		if lo < 0 {
			lo = 0
		}
		if hi >= nFrames {
			hi = nFrames - 1
		}
		var sum float64
		for j := lo; j <= hi; j++ {
			sum += flux[j]
		}
		mean := sum / float64(hi-lo+1)
		if flux[fi] < threshK*mean || flux[fi] < 1e-4 {
			continue
		}
		if fi-lastPick < minGapFrames {
			continue
		}
		times = append(times, float64(fi)*hopT)
		centroids = append(centroids, cents[fi])
		lastPick = fi
	}
	return times, centroids
}

// AssignLanes maps each onset to a lane 0..lanes-1 by quantile of its centroid.
func AssignLanes(centroids []float64, lanes int) []int {
	if len(centroids) == 0 {
		return nil
	}
	if lanes <= 1 {
		out := make([]int, len(centroids))
		return out
	}
	sorted := append([]float64(nil), centroids...)
	sort.Float64s(sorted)
	bounds := make([]float64, lanes-1)
	for i := 0; i < lanes-1; i++ {
		idx := (i + 1) * len(sorted) / lanes
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		bounds[i] = sorted[idx]
	}
	out := make([]int, len(centroids))
	for i, c := range centroids {
		lane := 0
		for _, b := range bounds {
			if c > b {
				lane++
			} else {
				break
			}
		}
		if lane >= lanes {
			lane = lanes - 1
		}
		out[i] = lane
	}
	return out
}

// EstimateBPM returns a tempo estimate from onset times by IOI median, folded to 60..180.
func EstimateBPM(times []float64) float64 {
	if len(times) < 4 {
		return 0
	}
	iois := make([]float64, 0, len(times)-1)
	for i := 1; i < len(times); i++ {
		d := times[i] - times[i-1]
		if d > 0.1 && d < 2.0 {
			iois = append(iois, d)
		}
	}
	if len(iois) == 0 {
		return 0
	}
	sort.Float64s(iois)
	med := iois[len(iois)/2]
	bpm := 60.0 / med
	for bpm < 60 {
		bpm *= 2
	}
	for bpm > 180 {
		bpm /= 2
	}
	return bpm
}
