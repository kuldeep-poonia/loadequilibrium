package collector

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Sample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

type Scraper struct {
	client           *http.Client
	maxBodyBytes     int64
	configuredTimout time.Duration
}

func NewScraper(timeout time.Duration, maxBodyBytes int64) *Scraper {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if maxBodyBytes <= 0 {
		maxBodyBytes = 4 << 20
	}
	return &Scraper{
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        128,
				MaxIdleConnsPerHost: 16,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		maxBodyBytes:     maxBodyBytes,
		configuredTimout: timeout,
	}
}

func (s *Scraper) Scrape(target ServiceTarget) ([]Sample, string, error) {
	if len(target.MetricsURLs) == 0 {
		return nil, "", fmt.Errorf("service %s has no metrics urls", target.ServiceID)
	}

	var lastErr error
	for _, endpoint := range target.MetricsURLs {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Accept", "text/plain; version=0.0.4, text/plain")
		req.Header.Set("User-Agent", "loadequilibrium-collector/1.0")

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, s.maxBodyBytes))
		closeErr := resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if closeErr != nil {
			lastErr = closeErr
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("%s returned status=%d", endpoint, resp.StatusCode)
			continue
		}
		samples, err := ParsePrometheusText(body)
		if err != nil {
			lastErr = err
			continue
		}
		return samples, endpoint, nil
	}
	return nil, "", lastErr
}

func ParsePrometheusText(data []byte) ([]Sample, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	samples := make([]Sample, 0, 256)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		token, rest, ok := splitMetricLine(line)
		if !ok {
			continue
		}
		valueField := firstField(rest)
		if valueField == "" {
			continue
		}
		value, err := strconv.ParseFloat(valueField, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}

		name, labels, err := parseMetricToken(token)
		if err != nil || name == "" {
			continue
		}
		samples = append(samples, Sample{Name: name, Labels: labels, Value: value})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

func splitMetricLine(line string) (string, string, bool) {
	inLabels := false
	inQuote := false
	escaped := false
	for i, r := range line {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inQuote:
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case r == '{' && !inQuote:
			inLabels = true
		case r == '}' && !inQuote:
			inLabels = false
		case unicode.IsSpace(r) && !inLabels && !inQuote:
			return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
		}
	}
	return "", "", false
}

func firstField(s string) string {
	for i, r := range s {
		if unicode.IsSpace(r) {
			return s[:i]
		}
	}
	return s
}

func parseMetricToken(token string) (string, map[string]string, error) {
	if open := strings.IndexByte(token, '{'); open >= 0 {
		close := strings.LastIndexByte(token, '}')
		if close < open {
			return "", nil, fmt.Errorf("invalid metric labels")
		}
		name := strings.TrimSpace(token[:open])
		labels := parseLabels(token[open+1 : close])
		return name, labels, nil
	}
	return strings.TrimSpace(token), nil, nil
}

func parseLabels(raw string) map[string]string {
	labels := make(map[string]string)
	i := 0
	for i < len(raw) {
		for i < len(raw) && (raw[i] == ',' || raw[i] == ' ' || raw[i] == '\t') {
			i++
		}
		start := i
		for i < len(raw) && raw[i] != '=' {
			i++
		}
		if i >= len(raw) {
			break
		}
		key := strings.TrimSpace(raw[start:i])
		i++
		if i >= len(raw) || raw[i] != '"' {
			break
		}
		i++
		var b strings.Builder
		for i < len(raw) {
			ch := raw[i]
			i++
			if ch == '\\' && i < len(raw) {
				next := raw[i]
				i++
				switch next {
				case 'n':
					b.WriteByte('\n')
				case '\\', '"':
					b.WriteByte(next)
				default:
					b.WriteByte(next)
				}
				continue
			}
			if ch == '"' {
				break
			}
			b.WriteByte(ch)
		}
		if key != "" {
			labels[key] = b.String()
		}
		for i < len(raw) && raw[i] != ',' {
			i++
		}
	}
	return labels
}
