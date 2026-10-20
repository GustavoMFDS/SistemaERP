package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type headerFlags []string

func (h *headerFlags) String() string { return strings.Join(*h, ",") }
func (h *headerFlags) Set(v string) error {
	*h = append(*h, v)
	return nil
}

type sample struct {
	duration time.Duration
	status   int
	err      error
}

func main() {
	var headers headerFlags
	url := flag.String("url", "", "target URL (required)")
	method := flag.String("method", http.MethodGet, "HTTP method")
	duration := flag.Duration("duration", 30*time.Second, "test duration")
	concurrency := flag.Int("concurrency", 8, "number of concurrent workers")
	timeout := flag.Duration("timeout", 5*time.Second, "per-request timeout")
	maxErrorRate := flag.Float64("max-error-rate", 0.01, "maximum accepted request error rate [0..1]")
	maxP95 := flag.Duration("max-p95", time.Second, "maximum accepted p95 latency")
	bodyFile := flag.String("body-file", "", "optional request body file")
	allowWrites := flag.Bool("allow-writes", false, "allow methods other than GET/HEAD")
	insecure := flag.Bool("insecure", false, "skip TLS verification (local drill only)")
	flag.Var(&headers, "header", "request header in 'Name: value' form; repeatable")
	flag.Parse()

	if err := run(config{
		URL:          *url,
		Method:       strings.ToUpper(strings.TrimSpace(*method)),
		Duration:     *duration,
		Concurrency:  *concurrency,
		Timeout:      *timeout,
		MaxErrorRate: *maxErrorRate,
		MaxP95:       *maxP95,
		BodyFile:     *bodyFile,
		AllowWrites:  *allowWrites,
		Insecure:     *insecure,
		Headers:      headers,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "loadcheck:", err)
		os.Exit(1)
	}
}

type config struct {
	URL          string
	Method       string
	Duration     time.Duration
	Concurrency  int
	Timeout      time.Duration
	MaxErrorRate float64
	MaxP95       time.Duration
	BodyFile     string
	AllowWrites  bool
	Insecure     bool
	Headers      []string
}

func run(cfg config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}

	body, err := readBody(cfg.BodyFile)
	if err != nil {
		return err
	}
	headers, err := parseHeaders(cfg.Headers)
	if err != nil {
		return err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.Insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit local-drill flag
	}
	client := &http.Client{Transport: transport, Timeout: cfg.Timeout}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	results := make(chan sample, cfg.Concurrency*4)
	var wg sync.WaitGroup
	wg.Add(cfg.Concurrency)
	for i := 0; i < cfg.Concurrency; i++ {
		go func() {
			defer wg.Done()
			worker(ctx, client, cfg, body, headers, results)
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var durations []time.Duration
	var total, failed int
	statusCounts := map[int]int{}
	started := time.Now()
	for s := range results {
		total++
		durations = append(durations, s.duration)
		if s.status != 0 {
			statusCounts[s.status]++
		}
		if s.err != nil || s.status < 200 || s.status >= 400 {
			failed++
		}
	}
	elapsed := time.Since(started)

	if total == 0 {
		return fmt.Errorf("no requests completed")
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := percentile(durations, 0.50)
	p95 := percentile(durations, 0.95)
	p99 := percentile(durations, 0.99)
	errorRate := float64(failed) / float64(total)
	rps := float64(total) / elapsed.Seconds()

	fmt.Printf("requests=%d failed=%d error_rate=%.4f rps=%.2f elapsed=%s\n", total, failed, errorRate, rps, elapsed.Round(time.Millisecond))
	fmt.Printf("latency p50=%s p95=%s p99=%s max=%s\n", p50.Round(time.Millisecond), p95.Round(time.Millisecond), p99.Round(time.Millisecond), durations[len(durations)-1].Round(time.Millisecond))
	fmt.Printf("statuses=%s\n", formatStatuses(statusCounts))

	if errorRate > cfg.MaxErrorRate {
		return fmt.Errorf("error rate %.4f exceeds threshold %.4f", errorRate, cfg.MaxErrorRate)
	}
	if p95 > cfg.MaxP95 {
		return fmt.Errorf("p95 latency %s exceeds threshold %s", p95, cfg.MaxP95)
	}
	fmt.Println("PASS: load thresholds satisfied")
	return nil
}

func validateConfig(cfg config) error {
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("-url is required")
	}
	if cfg.Concurrency < 1 || cfg.Concurrency > 1000 {
		return fmt.Errorf("-concurrency must be between 1 and 1000")
	}
	if cfg.Duration <= 0 {
		return fmt.Errorf("-duration must be positive")
	}
	if cfg.Timeout <= 0 {
		return fmt.Errorf("-timeout must be positive")
	}
	if cfg.MaxErrorRate < 0 || cfg.MaxErrorRate > 1 {
		return fmt.Errorf("-max-error-rate must be between 0 and 1")
	}
	if cfg.MaxP95 <= 0 {
		return fmt.Errorf("-max-p95 must be positive")
	}
	if cfg.Method != http.MethodGet && cfg.Method != http.MethodHead && !cfg.AllowWrites {
		return fmt.Errorf("method %s requires -allow-writes", cfg.Method)
	}
	return nil
}

func readBody(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read body file: %w", err)
	}
	return b, nil
}

func parseHeaders(values []string) (http.Header, error) {
	h := make(http.Header)
	for _, raw := range values {
		name, value, ok := strings.Cut(raw, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("invalid -header %q; expected 'Name: value'", raw)
		}
		h.Add(strings.TrimSpace(name), strings.TrimSpace(value))
	}
	return h, nil
}

func worker(ctx context.Context, client *http.Client, cfg config, body []byte, headers http.Header, out chan<- sample) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		var reader io.Reader
		if len(body) > 0 {
			reader = strings.NewReader(string(body))
		}
		req, err := http.NewRequestWithContext(ctx, cfg.Method, cfg.URL, reader)
		if err != nil {
			out <- sample{err: err}
			return
		}
		req.Header = headers.Clone()
		started := time.Now()
		resp, err := client.Do(req)
		took := time.Since(started)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			out <- sample{duration: took, err: err}
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		out <- sample{duration: took, status: resp.StatusCode}
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func formatStatuses(m map[int]int) string {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d:%d", k, m[k]))
	}
	return strings.Join(parts, ",")
}
