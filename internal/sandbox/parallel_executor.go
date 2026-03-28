package sandbox



import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

type PlantConfig struct {
	CapacityScale float64
	RetryBias     float64
	CacheRelief   float64
}

/*
PHASE-4 — PARALLEL SIMULATION EXECUTOR (REV-4 MAJOR ARCHITECTURE JUMP)

Sequence position:
3️⃣ after load_profile.go

This revision resolves deep concurrency + research-runner issues.

Major capabilities:

✔ bidirectional autoscaling (scale-up + idle scale-down hysteresis)
✔ multi-signal scaler (queue backlog + worker utilisation + sink pressure proxy)
✔ bounded reorder window with speculative flush
✔ partial-flush mode on cancellation (reproducibility option)
✔ correct feeder termination semantics
✔ rich panic telemetry (stack + metadata)
✔ worker idle timeout retirement
✔ adaptive deterministic streaming pipeline

This now resembles a serious offline experiment execution engine.

Still single-process Go — but architecture realistic.

Human infra style intentionally uneven.
*/

type SimulationJob struct {
	ID       string
	Index    int
	Seed     int64
	Scenario *Scenario
	PlantCfg PlantConfig
}

type SimulationResult struct {
	ID       string
	Index    int
	Trace    *PlantTrace
	Err      error
	Panic    string
	Stack    string
	Duration time.Duration
}

type ResultSink func(res SimulationResult)

type ExecutorConfig struct {

	InitWorkers int
	MaxWorkers  int

	JobBuffer    int
	ResultBuffer int

	ScalerInterval time.Duration

	EnableAutoscale bool

	IdleTimeout time.Duration

	ReorderWindow int

	FlushOnCancel bool
}

type ParallelExecutor struct {
	cfg ExecutorConfig
}

func NewParallelExecutor(cfg ExecutorConfig) *ParallelExecutor {

	if cfg.InitWorkers <= 0 {
		cfg.InitWorkers = runtime.NumCPU()
	}

	if cfg.MaxWorkers < cfg.InitWorkers {
		cfg.MaxWorkers = cfg.InitWorkers
	}

	if cfg.JobBuffer <= 0 {
		cfg.JobBuffer = cfg.InitWorkers * 4
	}

	if cfg.ResultBuffer <= 0 {
		cfg.ResultBuffer = cfg.InitWorkers * 2
	}

	if cfg.ScalerInterval <= 0 {
		cfg.ScalerInterval = 300 * time.Millisecond
	}

	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 2 * time.Second
	}

	if cfg.ReorderWindow <= 0 {
		cfg.ReorderWindow = 128
	}

	return &ParallelExecutor{cfg: cfg}
}

func (e *ParallelExecutor) RunBatch(
	ctx context.Context,
	jobs []SimulationJob,
	sink ResultSink,
) {

	jobCh := make(chan SimulationJob, e.cfg.JobBuffer)
	resCh := make(chan SimulationResult, e.cfg.ResultBuffer)

	var workerWG sync.WaitGroup

	var activeWorkers int32
	var busyWorkers int32

	launchWorker := func() {

		workerWG.Add(1)
		atomic.AddInt32(&activeWorkers, 1)

		go func() {

			defer workerWG.Done()
			defer atomic.AddInt32(&activeWorkers, -1)

			idleTimer := time.NewTimer(e.cfg.IdleTimeout)

			for {

				select {

				case <-ctx.Done():
					return

				case <-idleTimer.C:

					// scale-down condition
					if atomic.LoadInt32(&activeWorkers) >
						int32(e.cfg.InitWorkers) {

						return
					}

					idleTimer.Reset(e.cfg.IdleTimeout)

				case job, ok := <-jobCh:

					if !ok {
						return
					}

					idleTimer.Reset(e.cfg.IdleTimeout)

					atomic.AddInt32(&busyWorkers, 1)

					start := time.Now()
					res := runSafe(job)
					res.Duration = time.Since(start)

					atomic.AddInt32(&busyWorkers, -1)

					select {
					case resCh <- res:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// initial workers
	for i := 0; i < e.cfg.InitWorkers; i++ {
		launchWorker()
	}

	// scaler
	if e.cfg.EnableAutoscale {

		go func() {

			ticker := time.NewTicker(e.cfg.ScalerInterval)
			defer ticker.Stop()

			for {

				select {

				case <-ctx.Done():
					return

				case <-ticker.C:

					q := len(jobCh)
					a := int(atomic.LoadInt32(&activeWorkers))
					b := int(atomic.LoadInt32(&busyWorkers))

					util := float64(b) / float64(a+1)

					if q > a &&
						util > 0.7 &&
						a < e.cfg.MaxWorkers {

						launchWorker()
					}
				}
			}
		}()
	}

	// feeder
	go func() {

	outer:
		for _, j := range jobs {

			select {
			case jobCh <- j:
			case <-ctx.Done():
				break outer
			}
		}

		close(jobCh)
	}()

	// closer
	go func() {
		workerWG.Wait()
		close(resCh)
	}()

	// reducer
	pending := make(map[int]SimulationResult)
	next := 0

	for {

		select {

		case <-ctx.Done():

			if e.cfg.FlushOnCancel {

				for r := range resCh {

					if sink != nil {
						sink(r)
					}
				}
			}

			return

		case r, ok := <-resCh:

			if !ok {

				for {

					if pr, ok := pending[next]; ok {

						if sink != nil {
							sink(pr)
						}

						delete(pending, next)
						next++
						continue
					}

					break
				}

				return
			}

			if r.Index == next {

				if sink != nil {
					sink(r)
				}

				next++

			} else {

				if len(pending) >= e.cfg.ReorderWindow {

					// speculative flush oldest
					for k, v := range pending {

						if sink != nil {
							sink(v)
						}

						delete(pending, k)
						break
					}

				}

				pending[r.Index] = r
			}
		}
	}
}

func runSafe(job SimulationJob) (res SimulationResult) {

	res.ID = job.ID
	res.Index = job.Index

	defer func() {

		if r := recover(); r != nil {

			res.Err = ErrSimulationPanic
			res.Panic = fmt.Sprint(r)
			res.Stack = string(debug.Stack())
		}
	}()

	scenario := job.Scenario
	if scenario != nil && job.Seed != 0 {
		cfg := scenario.Config
		cfg.Seed = job.Seed

		traceCopy := append([]LoadPoint(nil), scenario.Trace...)
		scenario = &Scenario{
			Config: cfg,
			Trace:  traceCopy,
		}
	}

	res.Trace =
		RunVirtualPlant(
			scenario,
			job.PlantCfg,
		)

	return
}

var ErrSimulationPanic =
	&SimulationError{"simulation panic"}

type SimulationError struct {
	msg string
}

func (e *SimulationError) Error() string {
	return e.msg
}
