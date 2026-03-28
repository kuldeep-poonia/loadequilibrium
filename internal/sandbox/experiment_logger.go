package sandbox

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

/*
PHASE-4 — EXPERIMENT TRACE LOGGER (REV-3 SAFE-APPEND / CLOCK-CONSISTENT / SIZE-GUARDED)

Sequence position:
8️⃣ final file of Phase-4

This revision fixes remaining durability + tooling issues:

✔ single-member gzip policy (rotate → then compress)
✔ rotation guarded by process-level file mutex
✔ atomic temp name derived from injected clock (not wall-clock drift)
✔ auto directory creation
✔ structured stage-tagged error wrapping
✔ per-record size guard to avoid IO / memory spikes

Design intent:

research-sweep safe artifact writer
deterministic + bounded + append-friendly.

Human infra style intentionally uneven.
*/

const ExperimentSchemaVersion = "phase4.v3"

var logMu sync.Mutex

type ExperimentClock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

type ExperimentLineage struct {

	ScenarioHash string
	WorkloadHash string
	PlantHash    string
}

type ExperimentRecord struct {

	SchemaVersion string

	Timestamp time.Time

	Context  ExperimentContext
	Lineage  ExperimentLineage
	Metadata ExperimentMetadata

	Comparison ComparisonResult
	Advice     PolicyRecommendation
}

type LoggerConfig struct {

	OutputPath string

	Append bool

	EnableGzip bool

	MaxBytes int64      // rotate threshold
	MaxRecordBytes int64 // single record guard

	Clock ExperimentClock
}

func WriteExperimentRecord(
	rec ExperimentRecord,
	cfg LoggerConfig,
) error {

	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}

	rec.SchemaVersion = ExperimentSchemaVersion
	rec.Timestamp = cfg.Clock.Now()

	data, err :=
		json.Marshal(rec)

	if err != nil {
		return wrapErr("logger.marshal_failed", err)
	}

	if cfg.MaxRecordBytes > 0 &&
		int64(len(data)) > cfg.MaxRecordBytes {

		return errors.New("logger.record_too_large")
	}

	// ensure directory exists
	if err := os.MkdirAll(
		filepath.Dir(cfg.OutputPath),
		0755,
	); err != nil {

		return wrapErr("logger.mkdir_failed", err)
	}

	logMu.Lock()
	defer logMu.Unlock()

	if cfg.Append {

		if err := rotateIfNeeded(cfg); err != nil {
			return err
		}

		return appendNDJSONSafe(cfg, data)
	}

	return atomicWriteSafe(cfg, data)
}

/*
----- safe append (single-member gzip policy) -----
*/

func appendNDJSONSafe(
	cfg LoggerConfig,
	data []byte,
) error {

	f, err :=
		os.OpenFile(
			cfg.OutputPath,
			os.O_CREATE|os.O_APPEND|os.O_WRONLY,
			0644,
		)

	if err != nil {
		return wrapErr("logger.open_failed", err)
	}

	defer f.Close()

	line := append(data, '\n')

	if _, err = f.Write(line); err != nil {
		return wrapErr("logger.append_write_failed", err)
	}

	// optional post-rotation compression handled separately
	return nil
}

/*
----- rotation with optional compression -----
*/

func rotateIfNeeded(
	cfg LoggerConfig,
) error {

	if cfg.MaxBytes <= 0 {
		return nil
	}

	info, err := os.Stat(cfg.OutputPath)
	if err != nil {
		return nil
	}

	if info.Size() < cfg.MaxBytes {
		return nil
	}

	ts :=
		cfg.Clock.Now().Format("20060102-150405")

	rotated :=
		fmt.Sprintf("%s.%s", cfg.OutputPath, ts)

	if err :=
		os.Rename(cfg.OutputPath, rotated); err != nil {

		return wrapErr("logger.rotate_failed", err)
	}

	if cfg.EnableGzip {

		if err :=
			compressFile(rotated); err != nil {

			return err
		}
	}

	return nil
}

func compressFile(
	path string,
) error {

	src, err := os.Open(path)
	if err != nil {
		return wrapErr("logger.compress_open_failed", err)
	}
	defer src.Close()

	dst, err := os.Create(path + ".gz")
	if err != nil {
		return wrapErr("logger.compress_create_failed", err)
	}
	defer dst.Close()

	gz := gzip.NewWriter(dst)

	if _, err = src.WriteTo(gz); err != nil {
		return wrapErr("logger.compress_write_failed", err)
	}

	if err = gz.Close(); err != nil {
		return wrapErr("logger.compress_close_failed", err)
	}

	return os.Remove(path)
}

/*
----- atomic write (clock-consistent temp name) -----
*/

func atomicWriteSafe(
	cfg LoggerConfig,
	data []byte,
) error {

	tmp :=
		fmt.Sprintf(
			"%s.tmp-%d",
			cfg.OutputPath,
			cfg.Clock.Now().UnixNano(),
		)

	f, err := os.Create(tmp)
	if err != nil {
		return wrapErr("logger.tmp_create_failed", err)
	}

	if _, err = f.Write(data); err != nil {
		f.Close()
		return wrapErr("logger.tmp_write_failed", err)
	}

	if err = f.Sync(); err != nil {
		f.Close()
		return wrapErr("logger.tmp_sync_failed", err)
	}

	if err = f.Close(); err != nil {
		return wrapErr("logger.tmp_close_failed", err)
	}

	if err :=
		os.Rename(tmp, cfg.OutputPath); err != nil {

		return wrapErr("logger.rename_failed", err)
	}

	return nil
}

/*
----- structured error wrapper -----
*/

func wrapErr(
	stage string,
	err error,
) error {

	return fmt.Errorf("%s: %w", stage, err)
}
