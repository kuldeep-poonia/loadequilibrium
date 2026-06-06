package telemetry

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"
)

func makePoint(id string, req float64) *MetricPoint {
	return &MetricPoint{
		ServiceID:   id,
		Timestamp:   time.Now(),
		RequestRate: req,
		ErrorRate:   0.01,
		Latency: LatencyStats{
			Mean: req,
			P50:  req * 0.8,
			P95:  req * 1.2,
			P99:  req * 1.5,
		},
		CPUUsage:    req,
		MemUsage:    req,
		ActiveConns: int64(req),
		QueueDepth:  int64(req),
	}
}

func TestRingBufferEmpty(t *testing.T) {
	rb := NewRingBuffer(8)

	if rb.Size() != 0 {
		t.Fatalf("expected empty size")
	}

	last := rb.Last()
	if !last.Timestamp.IsZero() {
		t.Fatalf("expected zero MetricPoint")
	}

	if rb.Snapshot() != nil {
		t.Fatalf("expected nil snapshot")
	}
}

func TestRingBufferAppendAndLast(t *testing.T) {
	rb := NewRingBuffer(8)

	rb.Append(makePoint("svc", 10))
	rb.Append(makePoint("svc", 20))
	rb.Append(makePoint("svc", 30))

	last := rb.Last()

	if last.RequestRate != 30 {
		t.Fatalf("expected last=30 got=%v", last.RequestRate)
	}

	if rb.Size() != 3 {
		t.Fatalf("expected size=3 got=%d", rb.Size())
	}
}

func TestRingBufferSnapshotOrder(t *testing.T) {
	rb := NewRingBuffer(10)

	for i := 1; i <= 5; i++ {
		rb.Append(makePoint("svc", float64(i)))
	}

	snap := rb.Snapshot()

	if len(snap) != 5 {
		t.Fatalf("expected 5 points got=%d", len(snap))
	}

	for i := range snap {
		expected := float64(i + 1)

		if snap[i].RequestRate != expected {
			t.Fatalf(
				"snapshot order broken idx=%d expected=%v got=%v",
				i,
				expected,
				snap[i].RequestRate,
			)
		}
	}
}

func TestRingBufferSnapshotLimit(t *testing.T) {
	rb := NewRingBuffer(10)

	for i := 1; i <= 10; i++ {
		rb.Append(makePoint("svc", float64(i)))
	}

	snap := rb.Snapshot(3)

	if len(snap) != 3 {
		t.Fatalf("expected 3 points got=%d", len(snap))
	}

	expected := []float64{8, 9, 10}

	for i := range expected {
		if snap[i].RequestRate != expected[i] {
			t.Fatalf(
				"expected=%v got=%v",
				expected[i],
				snap[i].RequestRate,
			)
		}
	}
}

func TestRingBufferWrapAround(t *testing.T) {
	rb := NewRingBuffer(4)

	for i := 1; i <= 10; i++ {
		rb.Append(makePoint("svc", float64(i)))
	}

	if rb.Size() != 4 {
		t.Fatalf("expected size=4 got=%d", rb.Size())
	}

	snap := rb.Snapshot()

	expected := []float64{7, 8, 9, 10}

	for i := range expected {
		if snap[i].RequestRate != expected[i] {
			t.Fatalf(
				"wraparound failed idx=%d expected=%v got=%v",
				i,
				expected[i],
				snap[i].RequestRate,
			)
		}
	}
}

func TestRingBufferSummaryStats(t *testing.T) {
	rb := NewRingBuffer(10)

	values := []float64{10, 20, 30, 40}

	for _, v := range values {
		p := makePoint("svc", v)
		p.Latency.Mean = v
		rb.Append(p)
	}

	stats := rb.SummaryStats()

	if stats.Count != 4 {
		t.Fatalf("expected count=4 got=%d", stats.Count)
	}

	if stats.MeanReqRate != 25 {
		t.Fatalf("expected mean=25 got=%v", stats.MeanReqRate)
	}

	if stats.MaxLatencyMs != 40 {
		t.Fatalf("expected max latency=40 got=%v", stats.MaxLatencyMs)
	}

	if math.Abs(stats.StdReqRate-11.180339887) > 0.01 {
		t.Fatalf("std deviation incorrect got=%v", stats.StdReqRate)
	}
}

func TestRingBufferTimestampBounds(t *testing.T) {
	rb := NewRingBuffer(10)

	oldest := time.Now().Add(-10 * time.Minute)

	for i := 0; i < 5; i++ {
		p := makePoint("svc", float64(i))

		p.Timestamp = oldest.Add(
			time.Duration(i) * time.Minute,
		)

		rb.Append(p)
	}

	stats := rb.SummaryStats()

	if !stats.OldestAt.Equal(oldest) {
		t.Fatalf("oldest timestamp incorrect")
	}

	if !stats.NewestAt.Equal(oldest.Add(4 * time.Minute)) {
		t.Fatalf("newest timestamp incorrect")
	}
}

func TestRingBufferDeepCopyOnAppend(t *testing.T) {
	rb := NewRingBuffer(4)

	p := &MetricPoint{
		ServiceID: "svc",
		Timestamp: time.Now(),
		UpstreamCalls: []UpstreamCall{
			{
				TargetServiceID: "db",
				CallRate:        100,
			},
		},
	}

	rb.Append(p)

	p.UpstreamCalls[0].CallRate = 99999

	last := rb.Last()

	if last.UpstreamCalls[0].CallRate == 99999 {
		t.Fatal("ring buffer did not deep copy upstream slice")
	}
}

func TestRingBufferSnapshotIsolation(t *testing.T) {
	rb := NewRingBuffer(4)

	p := &MetricPoint{
		ServiceID: "svc",
		Timestamp: time.Now(),
		UpstreamCalls: []UpstreamCall{
			{
				TargetServiceID: "db",
				CallRate:        10,
			},
		},
	}

	rb.Append(p)

	snap := rb.Snapshot()

	snap[0].UpstreamCalls[0].CallRate = 999

	again := rb.Snapshot()

	if again[0].UpstreamCalls[0].CallRate == 999 {
		t.Fatal(
			"snapshot leaked internal memory (deep-copy bug)",
		)
	}
}

func TestRingBufferConcurrentAppend(t *testing.T) {
	const (
		capacity   = 128
		goroutines = 32
		perWorker  = 500
	)

	rb := NewRingBuffer(capacity)

	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			for i := 0; i < perWorker; i++ {
				rb.Append(
					makePoint(
						fmt.Sprintf("svc-%d", id),
						float64(i),
					),
				)
			}
		}(g)
	}

	wg.Wait()

	if rb.Size() != capacity {
		t.Fatalf(
			"expected ring to be full size=%d got=%d",
			capacity,
			rb.Size(),
		)
	}

	snap := rb.Snapshot()

	if len(snap) != capacity {
		t.Fatalf(
			"expected snapshot len=%d got=%d",
			capacity,
			len(snap),
		)
	}
}

func TestStoreHasServices(t *testing.T) {
	store := NewStore(8, 10, time.Minute)

	if store.HasServices() {
		t.Fatal("new store should be empty")
	}

	store.Ingest(makePoint("svc-a", 10))

	if !store.HasServices() {
		t.Fatal("expected services after ingest")
	}
}

func TestStoreServiceIDs(t *testing.T) {
	store := NewStore(8, 10, time.Minute)

	store.Ingest(makePoint("svc-a", 10))
	store.Ingest(makePoint("svc-b", 20))
	store.Ingest(makePoint("svc-c", 30))

	ids := store.ServiceIDs()

	if len(ids) != 3 {
		t.Fatalf("expected 3 services got=%d", len(ids))
	}
}

func TestStoreMaxServiceLimit(t *testing.T) {
	store := NewStore(8, 2, time.Minute)

	store.Ingest(makePoint("svc-a", 10))
	store.Ingest(makePoint("svc-b", 20))
	store.Ingest(makePoint("svc-c", 30))

	ids := store.ServiceIDs()

	if len(ids) > 2 {
		t.Fatalf("service limit violated ids=%d", len(ids))
	}
}

func TestWindowBasicMetrics(t *testing.T) {
	store := NewStore(16, 10, time.Minute)

	store.Ingest(makePoint("checkout", 10))
	store.Ingest(makePoint("checkout", 20))
	store.Ingest(makePoint("checkout", 30))

	w := store.Window("checkout", 10, 0)

	if w == nil {
		t.Fatal("expected window")
	}

	if w.SampleCount != 3 {
		t.Fatalf("expected sample count=3 got=%d", w.SampleCount)
	}

	if math.Abs(w.MeanRequestRate-20) > 0.01 {
		t.Fatalf("mean incorrect got=%v", w.MeanRequestRate)
	}

	if w.LastRequestRate != 30 {
		t.Fatalf("last request incorrect got=%v", w.LastRequestRate)
	}
}

func TestWindowMissingService(t *testing.T) {
	store := NewStore(8, 10, time.Minute)

	w := store.Window("ghost", 10, 0)

	if w != nil {
		t.Fatal("expected nil window")
	}
}

func TestWindowFreshnessCutoff(t *testing.T) {
	store := NewStore(8, 10, time.Minute)

	p := makePoint("svc", 10)
	p.Timestamp = time.Now().Add(-10 * time.Minute)

	store.Ingest(p)

	w := store.Window(
		"svc",
		10,
		time.Minute,
	)

	if w != nil {
		t.Fatal("expected stale window to be rejected")
	}
}

func TestAppliedScalePropagation(t *testing.T) {
	store := NewStore(8, 10, time.Minute)

	store.Ingest(makePoint("svc", 10))

	store.SetAppliedScale("svc", 2.5)

	all := store.AllWindows(10, 0)

	w := all["svc"]

	if w == nil {
		t.Fatal("window missing")
	}

	if w.AppliedScale != 2.5 {
		t.Fatalf(
			"expected scale=2.5 got=%v",
			w.AppliedScale,
		)
	}
}

func TestAllWindows(t *testing.T) {
	store := NewStore(8, 10, time.Minute)

	store.Ingest(makePoint("svc-a", 10))
	store.Ingest(makePoint("svc-b", 20))
	store.Ingest(makePoint("svc-c", 30))

	all := store.AllWindows(10, 0)

	if len(all) != 3 {
		t.Fatalf(
			"expected 3 windows got=%d",
			len(all),
		)
	}
}

func TestAllWindowsFreshnessFiltering(t *testing.T) {
	store := NewStore(8, 10, time.Hour)

	oldPoint := makePoint("old", 10)
	oldPoint.Timestamp = time.Now().Add(-10 * time.Minute)

	newPoint := makePoint("new", 10)

	store.Ingest(oldPoint)
	store.Ingest(newPoint)

	all := store.AllWindows(
		10,
		time.Minute,
	)

	if _, ok := all["old"]; ok {
		t.Fatal("stale service leaked into window set")
	}

	if _, ok := all["new"]; !ok {
		t.Fatal("fresh service missing")
	}
}

func TestWindowUpstreamAggregation(t *testing.T) {
	store := NewStore(8, 10, time.Minute)

	p := makePoint("api", 100)

	p.UpstreamCalls = []UpstreamCall{
		{
			TargetServiceID: "db",
			CallRate:        100,
			ErrorRate:       0.10,
			LatencyMean:     20,
		},
	}

	store.Ingest(p)

	w := store.Window("api", 10, 0)

	if w == nil {
		t.Fatal("window missing")
	}

	edge, ok := w.UpstreamEdges["db"]

	if !ok {
		t.Fatal("db edge missing")
	}

	if edge.MeanCallRate != 100 {
		t.Fatalf(
			"call rate mismatch got=%v",
			edge.MeanCallRate,
		)
	}
}

func TestSanitizeNegativeValues(t *testing.T) {
	store := NewStore(8, 10, time.Minute)

	p := &MetricPoint{
		ServiceID:   "svc",
		RequestRate: -100,
		ErrorRate:   -1,
		CPUUsage:    -50,
		MemUsage:    -50,
		ActiveConns: -10,
		QueueDepth:  -20,
		Latency: LatencyStats{
			Mean: -100,
			P50:  -100,
			P95:  -100,
			P99:  -100,
		},
	}

	store.Ingest(p)

	w := store.Window("svc", 10, 0)

	if w == nil {
		t.Fatal("window missing")
	}

	if w.LastRequestRate < 0 {
		t.Fatal("negative request rate survived sanitization")
	}

	if w.LastErrorRate < 0 {
		t.Fatal("negative error rate survived sanitization")
	}
}

func TestPruneRemovesService(t *testing.T) {
	store := NewStore(
		8,
		10,
		time.Second,
	)

	p := makePoint("svc", 10)
	p.Timestamp = time.Now().Add(-5 * time.Second)

	store.Ingest(p)

	pruned := store.Prune(time.Now())

	if len(pruned) != 1 {
		t.Fatalf(
			"expected 1 pruned service got=%d",
			len(pruned),
		)
	}

	if store.HasServices() {
		t.Fatal("service should be gone")
	}
}


func TestStoreConcurrentAdmissionLimit(t *testing.T) {
	const (
		maxSvc    = 10
		goroutine = 100
	)

	store := NewStore(
		8,
		maxSvc,
		time.Minute,
	)

	var wg sync.WaitGroup

	for i := 0; i < goroutine; i++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			store.Ingest(&MetricPoint{
				ServiceID: fmt.Sprintf(
					"svc-%d",
					id,
				),
				Timestamp:   time.Now(),
				RequestRate: 1,
				Latency: LatencyStats{
					Mean: 1,
				},
			})
		}(i)
	}

	wg.Wait()

	ids := store.ServiceIDs()

	if len(ids) > maxSvc {
		t.Fatalf(
			"service admission race detected: expected <= %d got=%d",
			maxSvc,
			len(ids),
		)
	}
}


func TestStoreConcurrentAdmissionLimitStress(t *testing.T) {
	const (
		maxSvc = 50
		rounds = 100
	)

	for r := 0; r < rounds; r++ {

		store := NewStore(
			8,
			maxSvc,
			time.Minute,
		)

		var wg sync.WaitGroup

		for i := 0; i < 500; i++ {

			wg.Add(1)

			go func(id int) {
				defer wg.Done()

				store.Ingest(&MetricPoint{
					ServiceID: fmt.Sprintf(
						"svc-%d",
						id,
					),
					Timestamp:   time.Now(),
					RequestRate: 1,
					Latency: LatencyStats{
						Mean: 1,
					},
				})

			}(i)
		}

		wg.Wait()

		if got := len(store.ServiceIDs()); got > maxSvc {

			t.Fatalf(
				"round=%d limit violated expected<=%d got=%d",
				r,
				maxSvc,
				got,
			)
		}
	}
}


func TestStoreConcurrentPruneAndIngest(t *testing.T) {
	store := NewStore(
		32,
		1000,
		time.Millisecond,
	)

	stop := make(chan struct{})

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		for {
			select {
			case <-stop:
				return
			default:

				store.Ingest(&MetricPoint{
					ServiceID: fmt.Sprintf(
						"svc-%d",
						time.Now().UnixNano()%100,
					),
					Timestamp:   time.Now(),
					RequestRate: 1,
					Latency: LatencyStats{
						Mean: 1,
					},
				})
			}
		}
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := 0; i < 1000; i++ {
			store.Prune(time.Now())
		}
	}()

	time.Sleep(500 * time.Millisecond)

	close(stop)

	wg.Wait()
}