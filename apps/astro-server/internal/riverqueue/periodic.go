package riverqueue

import (
	"time"

	"github.com/riverqueue/river"
)

// periodicJobs returns the periodic job definitions for the River client.
func periodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
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
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return ReconcilerArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 24 * time.Hour,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(10*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return DriftCheckArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 10 * time.Minute,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(10*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return NsScanArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 10 * time.Minute,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	}
}
