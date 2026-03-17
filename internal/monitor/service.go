package monitor

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/lev/internet-monitor/internal/config"
	"github.com/lev/internet-monitor/internal/store"
)

type Service struct {
	cfg        config.Config
	store      *store.FileStore
	httpClient *http.Client
}

func NewService(cfg config.Config, fileStore *store.FileStore) *Service {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   cfg.RequestTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   cfg.RequestTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &Service{
		cfg:   cfg,
		store: fileStore,
		httpClient: &http.Client{
			Timeout:   cfg.RequestTimeout,
			Transport: transport,
		},
	}
}

func (s *Service) Run(ctx context.Context) {
	checkTicker := time.NewTicker(s.cfg.CheckInterval)
	defer checkTicker.Stop()

	var speedTicker *time.Ticker
	if s.cfg.EnableSpeedTest {
		speedTicker = time.NewTicker(s.cfg.SpeedTestInterval)
		defer speedTicker.Stop()
	}

	s.runCheck(ctx, false)

	for {
		select {
		case <-ctx.Done():
			return
		case <-checkTicker.C:
			s.runCheck(ctx, false)
		case <-speedTick(speedTicker):
			s.runCheck(ctx, true)
		}
	}
}

func speedTick(ticker *time.Ticker) <-chan time.Time {
	if ticker == nil {
		return nil
	}
	return ticker.C
}

func (s *Service) runCheck(parent context.Context, includeSpeed bool) {
	ctx, cancel := context.WithTimeout(parent, s.cfg.RequestTimeout+time.Second)
	defer cancel()

	result := s.probe(ctx, includeSpeed)
	if err := s.store.Append(result); err != nil {
		log.Printf("store append: %v", err)
	}
}

func (s *Service) probe(ctx context.Context, includeSpeed bool) store.Record {
	record := store.Record{
		Timestamp: time.Now().UTC(),
	}

	_, lanOK := tcpProbe(ctx, s.cfg.LANProbeAddress)
	tcpLatency, tcpOK := tcpProbe(ctx, s.cfg.TCPProbeAddress)
	httpLatency, httpStatus, httpOK := httpProbe(ctx, s.httpClient, s.cfg.HTTPProbeURL)
	dnsOK := dnsProbe(ctx, s.cfg.DNSProbeHost)

	record.LANOK = lanOK
	record.TCPOK = tcpOK
	record.HTTPOK = httpOK
	record.HTTPStatus = httpStatus
	record.DNSOK = dnsOK
	record.Internet = tcpOK || httpOK
	record.Status = classify(tcpOK, httpOK, dnsOK)
	record.Note = explain(record, s.cfg.LANProbeAddress != "")
	record.LatencyMS = chooseLatencyMS(tcpLatency, httpLatency)

	if includeSpeed && record.Internet {
		if speedMbps, ok := s.speedProbe(ctx); ok {
			record.SpeedMbps = speedMbps
		}
	}

	return record
}

func (s *Service) speedProbe(ctx context.Context) (float64, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.SpeedTestURL, nil)
	if err != nil {
		return 0, false
	}

	started := time.Now()
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return 0, false
	}

	written, err := io.CopyN(io.Discard, resp.Body, s.cfg.SpeedTestBytes)
	if err != nil && err != io.EOF {
		return 0, false
	}
	if written == 0 {
		return 0, false
	}

	elapsed := time.Since(started).Seconds()
	if elapsed <= 0 {
		return 0, false
	}

	megabits := float64(written*8) / 1_000_000
	return megabits / elapsed, true
}

func tcpProbe(ctx context.Context, address string) (time.Duration, bool) {
	if address == "" {
		return 0, false
	}
	started := time.Now()
	conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return 0, false
	}
	conn.Close()
	return time.Since(started), true
}

func httpProbe(ctx context.Context, client *http.Client, url string) (time.Duration, int, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, false
	}

	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	statusOK := resp.StatusCode >= 200 && resp.StatusCode < 400
	return time.Since(started), resp.StatusCode, statusOK
}

func dnsProbe(ctx context.Context, host string) bool {
	resolver := net.Resolver{}
	addrs, err := resolver.LookupHost(ctx, host)
	return err == nil && len(addrs) > 0
}

func classify(tcpOK, httpOK, dnsOK bool) string {
	switch {
	case httpOK && tcpOK:
		return "up"
	case tcpOK || httpOK || dnsOK:
		return "degraded"
	default:
		return "down"
	}
}

func explain(record store.Record, lanConfigured bool) string {
	switch record.Status {
	case "up":
		if !record.DNSOK {
			return "internet ok, but dns probe failed"
		}
		if lanConfigured && record.LANOK {
			return "internet reachable, lan probe ok"
		}
		return "internet reachable"
	case "degraded":
		switch {
		case lanConfigured && !record.LANOK && record.TCPOK:
			return "internet reachable, but local lan probe failed"
		case record.TCPOK && !record.HTTPOK:
			return "tcp reachable, http probe failed"
		case record.HTTPOK && !record.DNSOK:
			return "http reachable, dns lookup failed"
		case record.DNSOK && !record.TCPOK && !record.HTTPOK:
			return "dns works, but direct internet probes failed"
		default:
			return "partial connectivity"
		}
	default:
		if lanConfigured && record.LANOK {
			return "lan reachable, upstream internet likely down"
		}
		if lanConfigured && !record.LANOK {
			return "internet unreachable, local network may be down"
		}
		return "internet unreachable"
	}
}

func chooseLatencyMS(latencies ...time.Duration) int64 {
	for _, latency := range latencies {
		if latency > 0 {
			return latency.Milliseconds()
		}
	}
	return 0
}
