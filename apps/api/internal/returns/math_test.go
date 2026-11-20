package returns

import (
	"errors"
	"math"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func assertNear(t *testing.T, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("got %.10f, want %.10f (±%g)", got, want, tol)
	}
}

func TestXIRR(t *testing.T) {
	tests := []struct {
		name  string
		flows []Flow
		want  float64
	}{
		{
			// Reference example from the Excel XIRR documentation.
			name: "excel reference",
			flows: []Flow{
				{day("2008-01-01"), -10000},
				{day("2008-03-01"), 2750},
				{day("2008-10-30"), 4250},
				{day("2009-02-15"), 3250},
				{day("2009-04-01"), 2750},
			},
			want: 0.373362535,
		},
		{
			// 1000 → 1210 over 730 days: 1.21^(365/730) − 1 = 0.1.
			name:  "two flows growth",
			flows: []Flow{{day("2021-01-01"), -1000}, {day("2023-01-01"), 1210}},
			want:  0.1,
		},
		{
			// 1000 → 810 over 730 days: 0.81^(1/2) − 1 = −0.1.
			name:  "two flows loss",
			flows: []Flow{{day("2021-01-01"), -1000}, {day("2023-01-01"), 810}},
			want:  -0.1,
		},
		{
			// At r = 0.1: −1000 + 550/1.1 + 605/1.21 = −1000 + 500 + 500 = 0.
			name: "intermediate withdrawal",
			flows: []Flow{
				{day("2021-01-01"), -1000},
				{day("2022-01-01"), 550},
				{day("2023-01-01"), 605},
			},
			want: 0.1,
		},
		{
			name: "unsorted input",
			flows: []Flow{
				{day("2023-01-01"), 1210},
				{day("2021-01-01"), -1000},
			},
			want: 0.1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := XIRR(tc.flows)
			if err != nil {
				t.Fatal(err)
			}
			assertNear(t, got, tc.want, 1e-6)
		})
	}
}

func TestXIRR_VolatileMonth(t *testing.T) {
	// +50% in 30 days: 1.5^(365/30) − 1 ≈ 138, far above the old 100 cap.
	got, err := XIRR([]Flow{{day("2021-01-01"), -1000}, {day("2021-01-31"), 1500}})
	if err != nil {
		t.Fatal(err)
	}
	want := math.Pow(1.5, 365.0/30) - 1
	assertNear(t, got/want, 1, 1e-6)
}

func TestXIRR_Errors(t *testing.T) {
	tests := []struct {
		name  string
		flows []Flow
		want  error
	}{
		{"single flow", []Flow{{day("2021-01-01"), -1000}}, ErrInsufficientHistory},
		{"shorter than 30 days", []Flow{{day("2021-01-01"), -1000}, {day("2021-01-30"), 1100}}, ErrInsufficientHistory},
		{"no sign change", []Flow{{day("2021-01-01"), -1000}, {day("2022-01-01"), -500}}, ErrNoSignChange},
		{"zero value", []Flow{{day("2021-01-01"), -1000}, {day("2022-01-01"), 0}}, ErrNoSignChange},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := XIRR(tc.flows); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestXIRR_ExactlyThirtyDays(t *testing.T) {
	// 1000 → 1100 in 30 days: 1.1^(365/30) − 1.
	got, err := XIRR([]Flow{{day("2021-01-01"), -1000}, {day("2021-01-31"), 1100}})
	if err != nil {
		t.Fatalf("expected a value at the 30-day boundary: %v", err)
	}
	assertNear(t, got, math.Pow(1.1, 365.0/30)-1, 1e-6)
}

func TestTWR(t *testing.T) {
	tests := []struct {
		name   string
		points []ValuePoint
		flows  []Flow
		want   float64
	}{
		{
			name: "flows inside sub-periods",
			points: []ValuePoint{
				{day("2024-01-01"), 100},
				{day("2024-02-01"), 120},
				{day("2024-03-01"), 99},
			},
			flows: []Flow{
				{day("2023-12-01"), -70}, // before V_0: ignored
				{day("2024-01-15"), -10}, // r1 = (120 − 10) / 100 − 1 = 0.1
				{day("2024-02-10"), 22},  // r2 = (99 + 22) / 120 − 1
			},
			want: 1.1*121/120 - 1,
		},
		{
			// Deposit dated on the V_0 day is not part of V_0:
			// r1 = (130 − 20) / 100 − 1 = 0.1, r2 = 132 / 130 − 1.
			name: "flow on first snapshot day",
			points: []ValuePoint{
				{day("2024-01-01"), 100},
				{day("2024-02-01"), 130},
				{day("2024-03-01"), 132},
			},
			flows: []Flow{{day("2024-01-01"), -20}},
			want:  1.1*132/130 - 1,
		},
		{
			// Deposit of 50 dated on the 02-01 snapshot day is absent from it:
			// r1 = 110 / 100 − 1 = 0.1, r2 = (160 − 50) / 110 − 1 = 0.
			name: "flow on later snapshot day goes to next period",
			points: []ValuePoint{
				{day("2024-01-01"), 100},
				{day("2024-02-01"), 110},
				{day("2024-03-01"), 160},
			},
			flows: []Flow{{day("2024-02-01"), -50}},
			want:  0.1,
		},
		{
			// r1 = (1100 − 1000) / 100 − 1 = 0, r2 = 1210 / 1100 − 1 = 0.1.
			name: "flow larger than the starting value",
			points: []ValuePoint{
				{day("2024-01-01"), 100},
				{day("2024-02-01"), 1100},
				{day("2024-03-01"), 1210},
			},
			flows: []Flow{{day("2024-01-15"), -1000}},
			want:  0.1,
		},
		{
			// V_1 − F_1 = 500 − 1000 < 0 would flip the product's sign:
			// period 1 is skipped, r2 = 550 / 500 − 1 = 0.1.
			name: "negative period end is skipped",
			points: []ValuePoint{
				{day("2024-01-01"), 100},
				{day("2024-02-01"), 500},
				{day("2024-03-01"), 550},
			},
			flows: []Flow{{day("2024-01-15"), -1000}},
			want:  0.1,
		},
		{
			// Today's snapshot (100) predates the deposit of 50 made today; the
			// live point (150) includes it: r1 = 0, r2 = (150 − 50) / 100 − 1 = 0.
			name: "snapshot today plus live point",
			points: []ValuePoint{
				{day("2024-02-01"), 100},
				{day("2024-03-01"), 100},
				{day("2024-03-01"), 150},
			},
			flows: []Flow{{day("2024-03-01"), -50}},
			want:  0,
		},
		{
			// Period 1 starts at 0 and is skipped: r2 = 110 / 100 − 1.
			name: "non-positive start is skipped",
			points: []ValuePoint{
				{day("2024-01-01"), 0},
				{day("2024-02-01"), 100},
				{day("2024-03-01"), 110},
			},
			flows: []Flow{{day("2024-01-10"), -100}},
			want:  0.1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TWR(tc.points, tc.flows)
			if err != nil {
				t.Fatal(err)
			}
			assertNear(t, got, tc.want, 1e-12)
		})
	}
}

func TestTWR_Errors(t *testing.T) {
	if _, err := TWR([]ValuePoint{{day("2024-01-01"), 100}}, nil); !errors.Is(err, ErrTooFewPoints) {
		t.Fatalf("single point: err = %v", err)
	}
	if _, err := TWR([]ValuePoint{{day("2024-01-01"), 0}, {day("2024-02-01"), 0}}, nil); !errors.Is(err, ErrNoMeasurablePeriod) {
		t.Fatalf("all-zero: err = %v", err)
	}
}

func TestAnnualize(t *testing.T) {
	// 21% over two years: 1.21^(1/2) − 1 = 0.1.
	got, ok := Annualize(0.21, 730)
	if !ok {
		t.Fatal("expected ok")
	}
	assertNear(t, got, 0.1, 1e-12)

	got, ok = Annualize(0.05, 365)
	if !ok {
		t.Fatal("expected ok at exactly one year")
	}
	assertNear(t, got, 0.05, 1e-12)

	if _, ok := Annualize(0.05, 364); ok {
		t.Fatal("shorter than a year must not annualize")
	}
}
