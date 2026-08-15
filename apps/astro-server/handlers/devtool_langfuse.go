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

// devtoolUsage is one source's usage over a window, read from Langfuse. Cells are
// kept unfolded so a single fetch of the widest window can serve every narrower
// range without re-querying.
type devtoolUsage struct{ Cells []devtoolCell }

// fetchDevtoolUsage reads one source's usage from the account's Langfuse project.
//
// Grouping by tags and userId alongside a day time-dimension makes every row one
// (day, tag-set, developer) cell. Dev-tool traces are tagged by source and belong
// to no deployment, so the tag is the only scope available.
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
		Dimensions:    []langfuse.MetricsDimension{{Field: "tags"}, {Field: "userId"}},
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

// since narrows to cells on or after a UTC day boundary.
func (u devtoolUsage) since(day time.Time) devtoolUsage {
	cutoff := day.UTC().Format("2006-01-02")
	out := devtoolUsage{Cells: make([]devtoolCell, 0, len(u.Cells))}
	for _, c := range u.Cells {
		if c.Date >= cutoff {
			out.Cells = append(out.Cells, c)
		}
	}
	return out
}

func (u devtoolUsage) totals() devtoolBucket {
	var t devtoolBucket
	for _, c := range u.Cells {
		t = addBucket(t, c.devtoolBucket)
	}
	return t
}

func (u devtoolUsage) byDay() map[string]devtoolBucket {
	out := map[string]devtoolBucket{}
	for _, c := range u.Cells {
		if c.Date != "" {
			out[c.Date] = addBucket(out[c.Date], c.devtoolBucket)
		}
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
