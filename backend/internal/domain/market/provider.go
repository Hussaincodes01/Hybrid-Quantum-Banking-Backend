// Package market fetches live index levels.
//
// Sensex and Nifty have no free official API. Yahoo Finance's public quote
// endpoint carries both (^BSESN, ^NSEI) with no key and no signup, which is
// what this uses. It is unofficial and can change without notice, so nothing
// here is allowed to break a screen: every read is served from cache, refreshes
// happen in the background, and a failed refresh keeps the last good values.
package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Quote is one index or commodity level.
type Quote struct {
	Symbol        string    `json:"symbol"`
	Value         float64   `json:"value"`
	PreviousClose float64   `json:"previousClose"`
	ChangePercent float64   `json:"changePercent"`
	At            time.Time `json:"at"`
}

// Snapshot is everything the dashboard and market screens need.
type Snapshot struct {
	Sensex     Quote     `json:"sensex"`
	Nifty      Quote     `json:"nifty"`
	GoldPer10g Quote     `json:"goldPer10g"`
	RepoRate   float64   `json:"repoRate"`
	FetchedAt  time.Time `json:"fetchedAt"`
	// Live is false when every upstream call has failed and these are the
	// seeded fallback levels. Callers surface this rather than passing stale
	// numbers off as live.
	Live bool `json:"live"`
}

const (
	sensexSymbol = "^BSESN"
	niftySymbol  = "^NSEI"
	// Gold futures in USD; converted to INR per 10g below.
	goldSymbol = "GC=F"

	refreshInterval = 2 * time.Minute
	fetchTimeout    = 8 * time.Second
)

// fallback levels, used only until the first successful fetch. These are the
// values that used to be hardcoded in the service.
var fallback = Snapshot{
	Sensex:     Quote{Symbol: sensexSymbol, Value: 75180.42},
	Nifty:      Quote{Symbol: niftySymbol, Value: 22831.11},
	GoldPer10g: Quote{Symbol: goldSymbol, Value: 74250.00},
	RepoRate:   6.50,
	Live:       false,
}

// Provider serves a cached snapshot and refreshes it in the background.
//
// Reads never block on the network: a request during an outage gets the last
// good snapshot immediately rather than waiting out an HTTP timeout, which is
// what would otherwise stall the dashboard.
type Provider struct {
	client *http.Client

	mu      sync.RWMutex
	current Snapshot

	once sync.Once
	stop chan struct{}
}

func NewProvider() *Provider {
	return &Provider{
		client:  &http.Client{Timeout: fetchTimeout},
		current: fallback,
		stop:    make(chan struct{}),
	}
}

// Start kicks off background refreshes. Safe to call more than once.
func (p *Provider) Start() {
	p.once.Do(func() {
		go func() {
			p.refresh()
			ticker := time.NewTicker(refreshInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					p.refresh()
				case <-p.stop:
					return
				}
			}
		}()
	})
}

func (p *Provider) Close() {
	select {
	case <-p.stop:
	default:
		close(p.stop)
	}
}

// Snapshot returns the cached values. Always safe, always immediate.
func (p *Provider) Snapshot() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.current
}

func (p *Provider) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	quotes, err := p.fetch(ctx, sensexSymbol, niftySymbol, goldSymbol)
	if err != nil || len(quotes) == 0 {
		// Keep whatever we had. A refresh failure must not downgrade a good
		// snapshot to the fallback.
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	next := p.current
	next.FetchedAt = time.Now()
	if q, ok := quotes[sensexSymbol]; ok {
		next.Sensex = q
		next.Live = true
	}
	if q, ok := quotes[niftySymbol]; ok {
		next.Nifty = q
		next.Live = true
	}
	if q, ok := quotes[goldSymbol]; ok {
		next.GoldPer10g = goldToINRPer10g(q)
	}
	p.current = next
}

// chartResponse mirrors only the fields used, so an unrelated schema change
// upstream cannot break decoding.
//
// This reads /v8/finance/chart rather than /v7/finance/quote: as of 2026 the
// quote endpoint returns 401 Unauthorized without a session cookie and crumb,
// while the chart endpoint still serves index levels unauthenticated.
type chartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol             string  `json:"symbol"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				PreviousClose      float64 `json:"previousClose"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
				Currency           string  `json:"currency"`
			} `json:"meta"`
		} `json:"result"`
		Error any `json:"error"`
	} `json:"chart"`
}

// fetch pulls each symbol concurrently. The chart endpoint takes one symbol
// per request, so a slow or failing symbol must not hold up the others.
func (p *Provider) fetch(ctx context.Context, symbols ...string) (map[string]Quote, error) {
	type result struct {
		quote Quote
		err   error
	}

	results := make(chan result, len(symbols))
	var wg sync.WaitGroup
	for _, symbol := range symbols {
		wg.Add(1)
		go func(sym string) {
			defer wg.Done()
			q, err := p.fetchOne(ctx, sym)
			results <- result{quote: q, err: err}
		}(symbol)
	}
	wg.Wait()
	close(results)

	out := make(map[string]Quote, len(symbols))
	var lastErr error
	for r := range results {
		if r.err != nil {
			lastErr = r.err
			continue
		}
		out[r.quote.Symbol] = r.quote
	}
	if len(out) == 0 {
		if lastErr == nil {
			lastErr = errors.New("market upstream returned no quotes")
		}
		return nil, lastErr
	}
	return out, nil
}

func (p *Provider) fetchOne(ctx context.Context, symbol string) (Quote, error) {
	url := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=5d",
		urlEscape(symbol),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Quote{}, err
	}
	// Yahoo rejects requests without a browser-ish user agent.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FINIX/1.0)")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return Quote{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Quote{}, fmt.Errorf("market upstream returned %d for %s", resp.StatusCode, symbol)
	}

	var decoded chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Quote{}, err
	}
	if len(decoded.Chart.Result) == 0 {
		return Quote{}, fmt.Errorf("no chart result for %s", symbol)
	}

	meta := decoded.Chart.Result[0].Meta
	if meta.RegularMarketPrice <= 0 {
		return Quote{}, fmt.Errorf("no price for %s", symbol)
	}

	// previousClose is frequently absent on index charts; chartPreviousClose is
	// the reliable one, so fall back to it before giving up on a change figure.
	prev := meta.PreviousClose
	if prev <= 0 {
		prev = meta.ChartPreviousClose
	}

	change := 0.0
	if prev > 0 {
		change = (meta.RegularMarketPrice - prev) / prev * 100
	}

	return Quote{
		Symbol:        symbol,
		Value:         meta.RegularMarketPrice,
		PreviousClose: prev,
		ChangePercent: change,
		At:            time.Now(),
	}, nil
}

// goldToINRPer10g converts a USD/troy-ounce futures price to the INR per 10
// grams that Indian customers actually see quoted.
//
// The FX rate is approximate and not fetched: a wrong gold figure is far less
// consequential here than an extra upstream dependency that can fail.
const (
	usdPerINR       = 83.5
	gramsPerTroyOz  = 31.1035
	gramsQuotedUnit = 10
)

func goldToINRPer10g(q Quote) Quote {
	if q.Value <= 0 {
		return fallback.GoldPer10g
	}
	perGramUSD := q.Value / gramsPerTroyOz
	q.Value = perGramUSD * gramsQuotedUnit * usdPerINR
	if q.PreviousClose > 0 {
		q.PreviousClose = q.PreviousClose / gramsPerTroyOz * gramsQuotedUnit * usdPerINR
	}
	return q
}

// urlEscape percent-encodes the caret in index symbols without pulling in
// net/url for a single character class.
func urlEscape(s string) string {
	out := make([]byte, 0, len(s)+4)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '^' {
			out = append(out, '%', '5', 'E')
			continue
		}
		out = append(out, c)
	}
	return string(out)
}
