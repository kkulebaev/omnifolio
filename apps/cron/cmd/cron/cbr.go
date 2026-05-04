package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"golang.org/x/text/encoding/charmap"
)

const cbrDailyURL = "https://www.cbr.ru/scripts/XML_daily.asp"

type cbrValute struct {
	XMLName  xml.Name `xml:"Valute"`
	NumCode  string   `xml:"NumCode"`
	CharCode string   `xml:"CharCode"`
	Nominal  string   `xml:"Nominal"`
	Name     string   `xml:"Name"`
	Value    string   `xml:"Value"`
}

type cbrValCurs struct {
	XMLName xml.Name    `xml:"ValCurs"`
	Date    string      `xml:"Date,attr"`
	Valutes []cbrValute `xml:"Valute"`
}

type cbrClient struct {
	http *http.Client
}

func newCBRClient() *cbrClient {
	return &cbrClient{http: &http.Client{Timeout: httpTimeout}}
}

// fxRate is a single ccy/RUB pair. cbr.ru publishes everything against RUB.
type fxRate struct {
	Date    time.Time
	FromCcy string
	ToCcy   string // always "RUB"
	Rate    decimal.Decimal
}

// fetchDaily returns latest available rates from cbr.ru for the given day
// (zero time = today). The response is windows-1251 encoded XML; we decode it
// per Tinkoff's CharsetReader pattern.
func (c *cbrClient) fetchDaily(ctx context.Context, day time.Time) ([]fxRate, error) {
	u := cbrDailyURL
	if !day.IsZero() {
		u = fmt.Sprintf("%s?date_req=%s", u, day.Format("02/01/2006"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Omnifolio FX fetcher)")
	req.Header.Set("Accept", "application/xml,text/xml")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cbr status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var data cbrValCurs
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	dec.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		switch strings.ToLower(label) {
		case "windows-1251", "cp1251":
			return charmap.Windows1251.NewDecoder().Reader(input), nil
		}
		return input, nil
	}
	if err := dec.Decode(&data); err != nil {
		return nil, fmt.Errorf("xml: %w", err)
	}

	date, err := time.Parse("02.01.2006", data.Date)
	if err != nil {
		return nil, fmt.Errorf("parse date %q: %w", data.Date, err)
	}

	out := make([]fxRate, 0, len(data.Valutes))
	for _, v := range data.Valutes {
		raw := strings.ReplaceAll(v.Value, ",", ".")
		rate, err := decimal.NewFromString(raw)
		if err != nil {
			continue
		}
		nominal, err := decimal.NewFromString(v.Nominal)
		if err != nil || nominal.IsZero() {
			nominal = decimal.NewFromInt(1)
		}
		out = append(out, fxRate{
			Date:    date,
			FromCcy: strings.ToUpper(v.CharCode),
			ToCcy:   "RUB",
			Rate:    rate.Div(nominal),
		})
	}
	return out, nil
}
