package returns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/deposits"
	"github.com/kkulebaev/omnifolio/api/internal/fx"
	"github.com/kkulebaev/omnifolio/api/internal/portfolio"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

// RateUnavailableError means a cash flow could not be converted at all: no
// rate for the pair exists on any date. Skipping the flow would silently
// distort every metric, so the whole computation fails instead.
type RateUnavailableError struct {
	From, To string
}

func (e *RateUnavailableError) Error() string {
	return fmt.Sprintf("FX rate unavailable for %s→%s", e.From, e.To)
}

func (e *RateUnavailableError) Unwrap() error { return fx.ErrRateUnavailable }

type Returns struct {
	Currency       string
	CurrentValue   decimal.Decimal
	TotalDeposited decimal.Decimal
	TotalWithdrawn decimal.Decimal
	NetInvested    decimal.Decimal
	Profit         decimal.Decimal
	ProfitPct      *decimal.Decimal
	XIRR           *decimal.Decimal
	TWR            *decimal.Decimal
	TWRAnnualized  *decimal.Decimal
	TWRFrom        *time.Time
	FirstFlowDate  *time.Time
	FXApproximated bool
	// XIRRReason / TWRReason explain a nil XIRR / TWR.
	XIRRReason *Reason
	TWRReason  *Reason
}

// Reason tells the UI why a metric is unavailable.
type Reason string

const (
	ReasonNoFlows             Reason = "no_flows"
	ReasonInsufficientHistory Reason = "insufficient_history"
	ReasonNoSignChange        Reason = "no_sign_change"
	ReasonNoConvergence       Reason = "no_convergence"
	ReasonNoSnapshots         Reason = "no_snapshots"
	ReasonNoMeasurablePeriod  Reason = "no_measurable_period"
)

func reasonFor(err error) *Reason {
	var r Reason
	switch {
	case errors.Is(err, ErrInsufficientHistory):
		r = ReasonInsufficientHistory
	case errors.Is(err, ErrNoSignChange):
		r = ReasonNoSignChange
	case errors.Is(err, ErrTooFewPoints):
		r = ReasonNoSnapshots
	case errors.Is(err, ErrNoMeasurablePeriod):
		r = ReasonNoMeasurablePeriod
	default:
		r = ReasonNoConvergence
	}
	return &r
}

// SeriesPoint asks for the cumulative net invested amount on Date in Currency.
type SeriesPoint struct {
	Date     time.Time
	Currency string
}

type Service struct {
	q         *storage.Queries
	portfolio *portfolio.Service
	fx        *fx.Service
	log       *slog.Logger
}

func NewService(q *storage.Queries, p *portfolio.Service, fxSvc *fx.Service, log *slog.Logger) *Service {
	return &Service{q: q, portfolio: p, fx: fxSvc, log: log}
}

// clipToToday folds flows dated tomorrow (allowed by validation to absorb
// timezone skew) into today, so they are not lost beyond the live valuation.
func clipToToday(d, today time.Time) time.Time {
	if d.After(today) {
		return today
	}
	return d
}

var epoch = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

// Compute returns the user's performance in currency. See docs/design.md §3.10
// for the conventions.
func (s *Service) Compute(ctx context.Context, userID uuid.UUID, currency string) (Returns, error) {
	pf, err := s.portfolio.Compute(ctx, userID, currency)
	if err != nil {
		return Returns{}, fmt.Errorf("portfolio: %w", err)
	}
	currency = pf.Summary.DisplayCurrency
	current := pf.Summary.GrandTotal

	rows, err := s.q.ListDepositsByUser(ctx, userID)
	if err != nil {
		return Returns{}, fmt.Errorf("list deposits: %w", err)
	}

	conv := newConverter(s.fx)
	out := Returns{
		Currency:       currency,
		CurrentValue:   current,
		TotalDeposited: decimal.Zero,
		TotalWithdrawn: decimal.Zero,
	}
	today := todayUTC()
	flows := make([]Flow, 0, len(rows)+1)
	for _, d := range rows {
		date := clipToToday(d.Date.Time, today)
		amt, approx, err := conv.convert(ctx, d.Amount, d.Currency, currency, date)
		if errors.Is(err, fx.ErrRateUnavailable) {
			return Returns{}, &RateUnavailableError{From: d.Currency, To: currency}
		}
		if err != nil {
			return Returns{}, err
		}
		out.FXApproximated = out.FXApproximated || approx

		f := amt.InexactFloat64()
		if d.Kind == deposits.KindWithdrawal {
			out.TotalWithdrawn = out.TotalWithdrawn.Add(amt)
			flows = append(flows, Flow{Date: date, Amount: f})
		} else {
			out.TotalDeposited = out.TotalDeposited.Add(amt)
			flows = append(flows, Flow{Date: date, Amount: -f})
		}
		if out.FirstFlowDate == nil || date.Before(*out.FirstFlowDate) {
			out.FirstFlowDate = &date
		}
	}

	out.NetInvested = out.TotalDeposited.Sub(out.TotalWithdrawn)
	out.Profit = current.Sub(out.NetInvested)
	if out.NetInvested.Sign() > 0 {
		pct := out.Profit.Div(out.NetInvested).Round(6)
		out.ProfitPct = &pct
	}

	currentF := current.InexactFloat64()
	if len(flows) == 0 {
		r := ReasonNoFlows
		out.XIRRReason = &r
	} else {
		xirrFlows := append(append([]Flow{}, flows...), Flow{Date: today, Amount: currentF})
		if r, err := XIRR(xirrFlows); err == nil {
			out.XIRR = roundedPtr(r)
		} else {
			out.XIRRReason = reasonFor(err)
		}
	}

	snaps, err := s.q.ListPortfolioSnapshotsByDateRange(ctx, storage.ListPortfolioSnapshotsByDateRangeParams{
		UserID:         userID,
		SnapshotDate:   pgtype.Date{Time: epoch, Valid: true},
		SnapshotDate_2: pgtype.Date{Time: today, Valid: true},
	})
	if err != nil {
		return Returns{}, fmt.Errorf("list snapshots: %w", err)
	}
	points := make([]ValuePoint, 0, len(snaps)+1)
	for _, snap := range snaps {
		v, approx, err := s.snapshotValue(ctx, conv, snap, currency)
		if err != nil {
			return Returns{}, err
		}
		out.FXApproximated = out.FXApproximated || approx
		points = append(points, ValuePoint{Date: snap.SnapshotDate.Time, Value: v.InexactFloat64()})
	}
	// The live valuation is always its own last point, even next to today's
	// snapshot: flows dated today are typically not in that snapshot yet.
	points = append(points, ValuePoint{Date: today, Value: currentF})
	if twr, err := TWR(points, flows); err == nil {
		out.TWR = roundedPtr(twr)
		from := points[0].Date
		out.TWRFrom = &from
		if ann, ok := Annualize(twr, daysBetween(from, today)); ok {
			out.TWRAnnualized = roundedPtr(ann)
		}
	} else {
		out.TWRReason = reasonFor(err)
	}

	return out, nil
}

// NetInvestedSeries returns, for each point, deposits minus withdrawals dated
// strictly before point.Date — a snapshot is taken before same-day flows land,
// the same convention TWR uses — in point.Currency, each flow converted at the rate on
// its own date. Flows that cannot be converted at all are left out rather than
// failing the history chart; partial reports that this happened.
func (s *Service) NetInvestedSeries(ctx context.Context, userID uuid.UUID, points []SeriesPoint) (series []decimal.Decimal, partial bool, err error) {
	out := make([]decimal.Decimal, len(points))
	if len(points) == 0 {
		return out, false, nil
	}
	rows, err := s.q.ListDepositsByUser(ctx, userID)
	if err != nil {
		return nil, false, fmt.Errorf("list deposits: %w", err)
	}
	today := todayUTC()
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Date.Time.Before(rows[j].Date.Time) })

	conv := newConverter(s.fx)
	skipped := map[uuid.UUID]string{}
	for i, p := range points {
		sum := decimal.Zero
		for _, d := range rows {
			date := clipToToday(d.Date.Time, today)
			if !date.Before(p.Date) {
				break
			}
			amt, _, err := conv.convert(ctx, d.Amount, d.Currency, p.Currency, date)
			if errors.Is(err, fx.ErrRateUnavailable) {
				skipped[d.ID] = d.Currency + "→" + p.Currency
				continue
			}
			if err != nil {
				return nil, false, err
			}
			if d.Kind == deposits.KindWithdrawal {
				sum = sum.Sub(amt)
			} else {
				sum = sum.Add(amt)
			}
		}
		out[i] = sum
	}
	if len(skipped) > 0 {
		pairs := make([]string, 0, len(skipped))
		for _, pair := range skipped {
			pairs = append(pairs, pair)
		}
		sort.Strings(pairs)
		s.log.Warn("returns: flows without any FX rate left out of net invested series",
			"user_id", userID, "flows", len(skipped), "pairs", strings.Join(slices.Compact(pairs), ","))
	}
	return out, len(skipped) > 0, nil
}

// snapshotValue re-prices a snapshot from its native by_currency sums so that
// history stays comparable after a display-currency change. A currency without
// any rate is dropped, mirroring how portfolio.Compute leaves it out of
// grand_total.
func (s *Service) snapshotValue(ctx context.Context, conv *converter, snap storage.PortfolioSnapshot, currency string) (decimal.Decimal, bool, error) {
	native := map[string]string{}
	if len(snap.ByCurrency) > 0 {
		if err := json.Unmarshal(snap.ByCurrency, &native); err != nil {
			return decimal.Zero, false, fmt.Errorf("decode by_currency: %w", err)
		}
	}
	total := decimal.Zero
	approximated := false
	for ccy, raw := range native {
		amt, err := decimal.NewFromString(raw)
		if err != nil {
			return decimal.Zero, false, fmt.Errorf("parse by_currency[%s]: %w", ccy, err)
		}
		v, approx, err := conv.convert(ctx, amt, ccy, currency, snap.SnapshotDate.Time)
		if errors.Is(err, fx.ErrRateUnavailable) {
			continue
		}
		if err != nil {
			return decimal.Zero, false, err
		}
		approximated = approximated || approx
		total = total.Add(v)
	}
	return total, approximated, nil
}

type rateKey struct {
	from, to string
	day      time.Time
}

type rateHit struct {
	rate   decimal.Decimal
	approx bool
	err    error
}

// converter memoizes historical rates for one request: flows and snapshots
// repeatedly hit the same (pair, date).
type converter struct {
	fx    *fx.Service
	cache map[rateKey]rateHit
}

func newConverter(fxSvc *fx.Service) *converter {
	return &converter{fx: fxSvc, cache: map[rateKey]rateHit{}}
}

func (c *converter) convert(ctx context.Context, amount decimal.Decimal, from, to string, at time.Time) (decimal.Decimal, bool, error) {
	key := rateKey{from: strings.ToUpper(from), to: strings.ToUpper(to), day: at}
	hit, ok := c.cache[key]
	if !ok {
		hit.rate, hit.approx, hit.err = c.fx.GetRateAt(ctx, key.from, key.to, at)
		if hit.err != nil && !errors.Is(hit.err, fx.ErrRateUnavailable) {
			return decimal.Zero, false, hit.err
		}
		c.cache[key] = hit
	}
	if hit.err != nil {
		return decimal.Zero, false, hit.err
	}
	return amount.Mul(hit.rate), hit.approx, nil
}

func roundedPtr(f float64) *decimal.Decimal {
	d := decimal.NewFromFloat(f).Round(6)
	return &d
}

func todayUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
