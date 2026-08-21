package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

// devtoolBucket is one slice of a source's usage — a day, a developer, or a total.
type devtoolBucket struct {
	CostUSD  float64
	Tokens   int
	Requests int
}

func (b devtoolBucket) empty() bool {
	return b.CostUSD == 0 && b.Tokens == 0 && b.Requests == 0
}

// devtoolCell is one (day, developer) measurement.
type devtoolCell struct {
	Date  string // YYYY-MM-DD, UTC
	Email string
	devtoolBucket
}

// devtoolUsage is one source's usage over a window, read from Langfuse. Cells
// stay unfolded so one fetch answers both the per-developer breakdown and the
// window total, which have to agree or the roll-up loses spend.
type devtoolUsage struct{ Cells []devtoolCell }

// fetchDevtoolUsage reads one source's usage from the account's Langfuse project.
//
// Grouping by tags and userId alongside a day time-dimension makes every row one
// (day, developer) cell. Dev-tool traces are tagged by source and belong to no
// deployment, so the tag is the only scope available; hasTag re-checks the
// server-side filter because the tags dimension returns whole tag arrays.
func fetchDevtoolUsage(
	ctx context.Context,
	client *langfuse.Client,
	source string,
	from, to time.Time,
) (devtoolUsage, error) {
	var usage devtoolUsage

	resp, err := client.GetMetrics(ctx, langfuse.MetricsQuery{
		View: "traces",
		Metrics: []langfuse.MetricsQueryField{
			{Measure: "totalCost", Aggregation: "sum"},
			{Measure: "totalTokens", Aggregation: "sum"},
			{Measure: "count", Aggregation: "count"},
		},
		Dimensions: []langfuse.MetricsDimension{{Field: "tags"}, {Field: "userId"}},
		Filters: []langfuse.MetricsFilter{
			{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: []string{source}},
		},
		TimeDimension: &langfuse.TimeDimension{Granularity: "day"},
		FromTimestamp: from.UTC().Format(time.RFC3339),
		ToTimestamp:   to.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return usage, err
	}

	for _, row := range resp.Data {
		if !hasTag(row["tags"], source) {
			continue
		}
		cell := devtoolCell{
			Date: dateFromTimeDim(row[langfuseTimeDimensionKey]),
			devtoolBucket: devtoolBucket{
				CostUSD:  toFloat(row["sum_totalCost"]),
				Tokens:   toInt(row["sum_totalTokens"]),
				Requests: toInt(row["count_count"]),
			},
		}
		if cell.empty() {
			continue
		}
		cell.Email, _ = row["userId"].(string)
		usage.Cells = append(usage.Cells, cell)
	}
	return usage, nil
}

func (u devtoolUsage) totals() devtoolBucket {
	var t devtoolBucket
	for _, c := range u.Cells {
		t = addBucket(t, c.devtoolBucket)
	}
	return t
}

// byDate splits a multi-day fetch back into one usage per day. Cells already
// carry the day they fall in, so a range query answers every day in the window
// and the caller still writes each day separately.
func (u devtoolUsage) byDate() map[string]devtoolUsage {
	out := map[string]devtoolUsage{}
	for _, c := range u.Cells {
		day := out[c.Date]
		day.Cells = append(day.Cells, c)
		out[c.Date] = day
	}
	return out
}

func (u devtoolUsage) byUser() map[string]devtoolBucket {
	out := map[string]devtoolBucket{}
	for _, c := range u.Cells {
		if c.Email != "" {
			out[c.Email] = addBucket(out[c.Email], c.devtoolBucket)
		}
	}
	return out
}

// costUnavailable reports tokens present but priced at zero. Langfuse derives
// cost from a model definition, so an unpriced model yields a real zero that is
// otherwise indistinguishable from no usage.
func (u devtoolUsage) costUnavailable() bool {
	t := u.totals()
	return t.Tokens > 0 && t.CostUSD == 0
}

func addBucket(a, b devtoolBucket) devtoolBucket {
	return devtoolBucket{
		CostUSD:  a.CostUSD + b.CostUSD,
		Tokens:   a.Tokens + b.Tokens,
		Requests: a.Requests + b.Requests,
	}
}

func hasTag(raw any, want string) bool {
	for _, t := range tagStrings(raw) {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}
