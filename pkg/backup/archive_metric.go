package backup

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type ArchiveMetricArgs struct {
	Parent     Parent   `json:"parent"`
	MetricKeys []string `json:"metric_keys"`
}

func (ArchiveMetricArgs) Kind() string {
	return "archive_metric"
}

type ArchiveMetricWorker struct {
	river.WorkerDefaults[ArchiveMetricArgs]
	dbPool *pgxpool.Pool
}

func NewArchiveMetricWorker(dbPool *pgxpool.Pool) *ArchiveMetricWorker {
	return &ArchiveMetricWorker{dbPool: dbPool}
}

func (w *ArchiveMetricWorker) Run(ctx context.Context, job *river.Job[ArchiveMetricArgs]) error {

	tx, err := w.dbPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	return nil
}
