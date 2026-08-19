package riverqueue

import (
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
)

// periodicJobs returns the periodic job definitions for the River client.
func periodicJobs(cfg Config) []*river.PeriodicJob {
	jobs := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(5*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return MeteringArgs{}, &river.InsertOpts{
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

		// Observation alert sweep: evaluate resource/health conditions against
		// metrics and emit alerts on firing edges (state in deployment_alert_state).
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(5*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return ObservationSweepArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 5 * time.Minute,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	// Backfills accounts that never got a rate card or signup credit.
	if cfg.BillingBackend == "metronome" {
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(1*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return BillingProvisionSweepArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 1 * time.Hour,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	// Billing dunning-grace sweep (hosted/metronome only) — ages past_due
	// accounts to suspended once their grace window elapses.
	if cfg.BillingBackend == "metronome" {
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(1*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return DunningSweepArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 1 * time.Hour,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

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

	// Stuck-deployment watchdog: fail deployments wedged in pending/
	// provisioning/deploying past their per-status deadline (see
	// deploy_watchdog.go). Backstops the K8s progressDeadline, which only
	// bounds Deployment-kind rollouts and never covers pending/provisioning.
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(5*time.Minute),
		func() (river.JobArgs, *river.InsertOpts) {
			return DeploymentWatchdogArgs{}, &river.InsertOpts{
				UniqueOpts: river.UniqueOpts{
					ByPeriod: 5 * time.Minute,
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

	if cfg.DeploymentFGASync != nil && cfg.DeploymentFGASync.Enabled() {
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(1*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return DeploymentFGAReconcileArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 1 * time.Minute,
					},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	if cfg.ResourceAccessSync != nil && cfg.AccessReconciler != nil {
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(1*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return ResourceAccessFGAReconcileArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{ByPeriod: 1 * time.Minute},
				}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	if cfg.WorkOSClient != nil {
		// Backfill member emails still missing from the mirror. Emails are
		// captured at auth time (login + account create); this reconcile is the
		// safety net that fills any gaps by querying WorkOS directly.
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return MemberEmailReconcileArgs{}, &river.InsertOpts{
					UniqueOpts: river.UniqueOpts{
						ByPeriod: 24 * time.Hour,
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

	// Roll up completed days into the durable fact table. RunOnStart so a fresh
	// deployment begins backfilling immediately rather than a day later; the
	// watermark makes a restart cheap, since already-rolled days are skipped.
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(insightsrollup.RollupInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return InsightsRollupArgs{}, &river.InsertOpts{
				UniqueOpts: river.UniqueOpts{
					ByPeriod: insightsrollup.RollupInterval,
				},
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	// Label Claude Code prompts; separate from the roll-up so a Foundry
	// outage lags labels without stalling spend reporting.
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(ClassificationInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return ClassificationArgs{}, &river.InsertOpts{
				UniqueOpts: river.UniqueOpts{ByPeriod: ClassificationInterval},
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	return jobs
}
