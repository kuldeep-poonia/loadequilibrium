package collector

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type Collector struct {
	cfg        Config
	docker     *DockerClient
	scraper    *Scraper
	normalizer *Normalizer
	ingest     *IngestClient
	stats      *Stats
	targets    atomic.Value // []ServiceTarget
}

func New(cfg Config) (*Collector, error) {
	dockerClient, err := NewDockerClient(cfg.DockerHost)
	if err != nil {
		return nil, err
	}
	stats := &Stats{}
	c := &Collector{
		cfg:        cfg,
		docker:     dockerClient,
		scraper:    NewScraper(cfg.ScrapeTimeout, cfg.MaxScrapeBodyBytes),
		normalizer: NewNormalizer(),
		stats:      stats,
	}
	c.ingest = NewIngestClient(cfg, stats)
	c.targets.Store([]ServiceTarget{})
	return c, nil
}

func (c *Collector) Stats() *Stats {
	return c.stats
}

func (c *Collector) Targets() []ServiceTarget {
	targets, _ := c.targets.Load().([]ServiceTarget)
	out := make([]ServiceTarget, len(targets))
	copy(out, targets)
	return out
}

func (c *Collector) Run(ctx context.Context) {
	go c.ingest.Run(ctx)

	c.discover(ctx)
	c.scrape(ctx)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c.discoveryLoop(ctx)
	}()
	go func() {
		defer wg.Done()
		c.scrapeLoop(ctx)
	}()
	<-ctx.Done()
	wg.Wait()
}

func (c *Collector) discoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.DiscoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.discover(ctx)
		}
	}
}

func (c *Collector) scrapeLoop(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.ScrapeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.scrape(ctx)
		}
	}
}

func (c *Collector) discover(ctx context.Context) {
	targets, err := c.docker.Discover(ctx, c.cfg)
	now := time.Now()
	if err != nil {
		c.stats.markError(now)
		log.Printf(`{"component":"le-collector","level":"warn","event":"docker_discovery_failed","error":%q}`, err.Error())
		return
	}
	c.targets.Store(targets)
	c.stats.DiscoveredServices.Store(int64(len(targets)))
	c.stats.LastDiscoveryUnix.Store(now.Unix())
	log.Printf(`{"component":"le-collector","level":"info","event":"docker_discovery","services":%d}`, len(targets))
}

func (c *Collector) scrape(ctx context.Context) {
	targets := c.Targets()
	if len(targets) == 0 {
		return
	}

	limit := c.cfg.MaxScrapeConcurrency
	if limit <= 0 {
		limit = 16
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, target := range targets {
		target := target
		select {
		case <-ctx.Done():
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()
			c.scrapeOne(target)
		}()
	}
	wg.Wait()
	c.stats.LastScrapeUnix.Store(time.Now().Unix())
}

func (c *Collector) scrapeOne(target ServiceTarget) {
	samples, endpoint, err := c.scraper.Scrape(target)
	now := time.Now()
	if err != nil {
		c.stats.ScrapeErrorTotal.Add(1)
		c.stats.markError(now)
		log.Printf(`{"component":"le-collector","level":"warn","event":"scrape_failed","service":%q,"error":%q}`, target.ServiceID, err.Error())
		return
	}
	point, ok := c.normalizer.Build(target, samples, now)
	if !ok {
		c.stats.ScrapeSuccessTotal.Add(1)
		return
	}
	c.stats.ScrapeSuccessTotal.Add(1)
	c.stats.PointsBuiltTotal.Add(1)
	if !c.ingest.Enqueue(point) {
		log.Printf(`{"component":"le-collector","level":"warn","event":"point_dropped","service":%q}`, target.ServiceID)
		return
	}
	if len(point.UpstreamCalls) > 0 {
		log.Printf(`{"component":"le-collector","level":"debug","event":"point_queued","service":%q,"endpoint":%q,"rps":%.3f,"upstreams":%d}`, target.ServiceID, endpoint, point.RequestRate, len(point.UpstreamCalls))
	}
}
