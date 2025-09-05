// Package backup provides a backup service that sync from redis to postgres.
package backup

import "github.com/riverqueue/river/rivertype"

// Parent helps trace which job is the parent of the current job.
type Parent struct {
	Kind  string `json:"kind"`
	JobID int64  `json:"job_id"`
}

// Hydrate hydrates the parent from the job row.
func (p *Parent) Hydrate(job *rivertype.JobRow) {
	p.Kind = job.Kind
	p.JobID = job.ID
}
