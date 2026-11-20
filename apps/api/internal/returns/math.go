// Package returns computes portfolio performance (net invested, P&L, XIRR,
// TWR) from the deposits journal and daily portfolio snapshots.
// See docs/design.md §3.10.
package returns

import (
	"errors"
	"math"
	"sort"
	"time"
)

// Flow is an external cash flow from the investor's point of view: money put
// into the portfolio is negative, money taken out is positive. XIRR expects the
// current portfolio value as a final positive flow dated today.
type Flow struct {
	Date   time.Time
	Amount float64
}

// ValuePoint is the portfolio value on a given day.
type ValuePoint struct {
	Date  time.Time
	Value float64
}

var (
	ErrInsufficientHistory = errors.New("returns: flows span less than 30 days")
	ErrNoSignChange        = errors.New("returns: flows have no sign change")
	ErrNoConvergence       = errors.New("returns: solver did not converge")
	ErrTooFewPoints        = errors.New("returns: fewer than two value points")
	ErrNoMeasurablePeriod  = errors.New("returns: no measurable sub-period")
)

// An annualized rate over a few weeks extrapolates noise into absurd numbers
// (+5% in 10 days reads as +500%/yr), so XIRR is withheld below this window.
const minXIRRDays = 30

const (
	xirrLow = -0.9999
	// Wide enough for a volatile month (+50% in 30 days ≈ +13 800%/yr) to
	// still bracket the root.
	xirrHigh    = 1e6
	xirrTol     = 1e-10
	xirrMaxIter = 300
)

// XIRR solves Σ a_i / (1+r)^(t_i/365) = 0 for r (ACT/365) with Newton's method,
// falling back to bisection over [-0.9999, 1e6].
func XIRR(flows []Flow) (float64, error) {
	if len(flows) < 2 {
		return 0, ErrInsufficientHistory
	}
	sorted := sortedFlows(flows)
	first := sorted[0].Date
	last := sorted[len(sorted)-1].Date
	if daysBetween(first, last) < minXIRRDays {
		return 0, ErrInsufficientHistory
	}

	var hasNeg, hasPos bool
	years := make([]float64, len(sorted))
	amounts := make([]float64, len(sorted))
	for i, f := range sorted {
		years[i] = float64(daysBetween(first, f.Date)) / 365
		amounts[i] = f.Amount
		if f.Amount < 0 {
			hasNeg = true
		}
		if f.Amount > 0 {
			hasPos = true
		}
	}
	if !hasNeg || !hasPos {
		return 0, ErrNoSignChange
	}

	npv := func(r float64) (v, dv float64) {
		for i, a := range amounts {
			d := math.Pow(1+r, years[i])
			v += a / d
			dv -= years[i] * a / (d * (1 + r))
		}
		return v, dv
	}

	r := 0.1
	for range xirrMaxIter {
		v, dv := npv(r)
		if dv == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			break
		}
		next := r - v/dv
		if math.IsNaN(next) || next <= -1 || next > xirrHigh {
			break
		}
		if math.Abs(next-r) < xirrTol*math.Max(1, math.Abs(r)) {
			return next, nil
		}
		r = next
	}

	lo, hi := xirrLow, xirrHigh
	vlo, _ := npv(lo)
	vhi, _ := npv(hi)
	if math.IsNaN(vlo) || math.IsNaN(vhi) || math.Signbit(vlo) == math.Signbit(vhi) {
		return 0, ErrNoConvergence
	}
	for range xirrMaxIter {
		mid := (lo + hi) / 2
		vmid, _ := npv(mid)
		if math.Signbit(vmid) == math.Signbit(vlo) {
			lo, vlo = mid, vmid
		} else {
			hi = mid
		}
		if hi-lo < xirrTol*math.Max(1, math.Abs(lo)) {
			return (lo + hi) / 2, nil
		}
	}
	return 0, ErrNoConvergence
}

// TWR chains sub-period returns between neighbouring value points.
//
// Snapshots are taken at a fixed time of day while flows carry only a date, so
// a flow dated d may or may not be inside the snapshot of day d. It is assigned
// to the sub-period ending at the first point dated strictly after d, i.e. it is
// assumed absent from V_d and present in the next value. The last point is the
// live valuation and absorbs every flow not assigned earlier. Flows dated before
// the first point are ignored (TWR starts at V_0).
//
// r_i = (V_i − F_i) / V_{i-1} − 1 with F_i the net contribution (deposits minus
// withdrawals, i.e. −Σ Flow.Amount). A sub-period with V_{i-1} ≤ 0 or
// V_i − F_i ≤ 0 is skipped so the product never flips sign.
func TWR(points []ValuePoint, flows []Flow) (float64, error) {
	if len(points) < 2 {
		return 0, ErrTooFewPoints
	}
	pts := make([]ValuePoint, len(points))
	copy(pts, points)
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].Date.Before(pts[j].Date) })

	contributions := make([]float64, len(pts))
	for _, f := range flows {
		if f.Date.Before(pts[0].Date) {
			continue
		}
		// Searching all but the last point: when none is strictly later, the
		// index lands on the live point.
		i := sort.Search(len(pts)-1, func(k int) bool { return pts[k].Date.After(f.Date) })
		contributions[i] -= f.Amount
	}

	growth := 1.0
	measured := false
	for i := 1; i < len(pts); i++ {
		prev := pts[i-1].Value
		end := pts[i].Value - contributions[i]
		if prev <= 0 || end <= 0 {
			continue
		}
		growth *= end / prev
		measured = true
	}
	if !measured {
		return 0, ErrNoMeasurablePeriod
	}
	return growth - 1, nil
}

// Annualize converts a cumulative return over `days` into a yearly rate. It
// refuses periods shorter than a year, where compounding up would overstate
// what was actually earned.
func Annualize(cumulative float64, days int) (float64, bool) {
	if days < 365 || cumulative <= -1 {
		return 0, false
	}
	return math.Pow(1+cumulative, 365/float64(days)) - 1, true
}

func sortedFlows(flows []Flow) []Flow {
	out := make([]Flow, len(flows))
	copy(out, flows)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

func daysBetween(a, b time.Time) int {
	return int(math.Round(b.Sub(a).Hours() / 24))
}
