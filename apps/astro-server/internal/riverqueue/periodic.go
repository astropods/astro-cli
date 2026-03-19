package riverqueue

import (
	"time"

	"github.com/riverqueue/river"
)

// periodicJobs returns the periodic job definitions for the River client.
func periodicJobs(cfg Config) []*river.PeriodicJob {
	jobs := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(5*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return HeartbeatArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 5 * time.Minute,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	}

	if cfg.PromClient != nil {
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(5*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return MessageCountSyncArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 5 * time.Minute,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(10*time.Minute),
		func() (river.JobArgs, *river.InsertOpts) {
			return ReconcileArgs{}, &river.InsertOpts{
				UniqueOpts: river.UniqueOpts{
					ByPeriod: 10 * time.Minute,
				},
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	if cfg.WorkOSAPIKey != "" {
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(30*time.Second),
			func() (river.JobArgs, *river.InsertOpts) {
				return WorkOSEventsArgs{}, &river.InsertOpts{
					Queue: queueWorkOS,
					UniqueOpts: river.UniqueOpts{
						ByQueue: true,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	return jobs
}
