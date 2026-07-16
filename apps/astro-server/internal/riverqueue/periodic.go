package riverqueue

import (
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
)

// periodicJobs returns the periodic job definitions for the River client.
func periodicJobs(cfg Config) []*river.PeriodicJob {
	jobs := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(5*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return OpenmeterArgs{}, &river.InsertOpts{
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

	// Drift / reconcile periodic job is paused while the env model
	// migrates to deployment_build_env. The reconcile worker reads
	// deployment_resolved_keys, which is going away in the cleanup
	// flow. Re-enable once a row-based drift check exists.
	//
	// jobs = append(jobs, river.NewPeriodicJob(
	// 	river.PeriodicInterval(10*time.Minute),
	// 	func() (river.JobArgs, *river.InsertOpts) {
	// 		return ReconcileArgs{}, &river.InsertOpts{
	// 			UniqueOpts: river.UniqueOpts{
	// 				ByPeriod: 10 * time.Minute,
	// 			},
	// 		}
	// 	},
	// 	&river.PeriodicJobOpts{RunOnStart: true},
	// ))

	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(30*time.Second),
		func() (river.JobArgs, *river.InsertOpts) {
			return KnowledgeReconcileArgs{}, &river.InsertOpts{
				UniqueOpts: river.UniqueOpts{
					ByPeriod: 30 * time.Second,
				},
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	if cfg.AvatarStore != nil {
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return AvatarBackfillArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 24 * time.Hour,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))

		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return BlueprintAvatarBackfillArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 24 * time.Hour,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return ProviderBackfillArgs{}, &river.InsertOpts{
				UniqueOpts: river.UniqueOpts{
					ByPeriod: 24 * time.Hour,
				},
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	if cfg.Billing != nil {
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return OpenMeterBackfillArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 24 * time.Hour,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	if cfg.WorkOSAPIKey != "" {
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(15*time.Second),
			func() (river.JobArgs, *river.InsertOpts) {
				return WorkOSEventsArgs{}, &river.InsertOpts{
					Queue: queueWorkOS,
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 15 * time.Second,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(1*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return AccountPurgeArgs{}, &river.InsertOpts{
				UniqueOpts: river.UniqueOpts{
					ByPeriod: 1 * time.Hour,
				},
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	// Refresh the observability summary cache on the same interval the
	// frontend's TTL admits. RunOnStart so a fresh server populates the
	// cache immediately rather than waiting RefreshInterval for the first
	// page-load to have data.
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(obssummary.RefreshInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return ObsSummaryRefreshArgs{}, &river.InsertOpts{
				UniqueOpts: river.UniqueOpts{
					ByPeriod: obssummary.RefreshInterval,
				},
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	// Pre-warm the Insights endpoint cache. The handler reads from the
	// same cache on every request, so a server-cold start blocks the first
	// page-load on a live Langfuse fan-out until this finishes — RunOnStart
	// gets us into the steady-state path sooner.
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(insightscache.RefreshInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return InsightsRefreshArgs{}, &river.InsertOpts{
				UniqueOpts: river.UniqueOpts{
					ByPeriod: insightscache.RefreshInterval,
				},
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	return jobs
}
