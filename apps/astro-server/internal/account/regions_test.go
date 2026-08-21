package account

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
)

func TestRegionLabel(t *testing.T) {
	tests := []struct {
		region string
		want   string
	}{
		{"us-east-1", "US East (N. Virginia)"},
		{"eu-west-1", "Europe (Ireland)"},
		{"ap-southeast-2", "Asia Pacific (Sydney)"},
		{"xx-nowhere-9", "xx-nowhere-9"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := RegionLabel(tc.region); got != tc.want {
			t.Errorf("RegionLabel(%q) = %q, want %q", tc.region, got, tc.want)
		}
	}
}

func TestRegionFlag(t *testing.T) {
	tests := []struct {
		region string
		want   string
	}{
		{"eu-west-1", "🇮🇪"},
		{"us-west-2", "🇺🇸"},
		{"ap-northeast-1", "🇯🇵"},
		{"xx-nowhere-9", ""},
		{"", ""},
	}
	for _, tc := range tests {
		if got := RegionFlag(tc.region); got != tc.want {
			t.Errorf("RegionFlag(%q) = %q, want %q", tc.region, got, tc.want)
		}
	}
}

func TestRegionsAreComplete(t *testing.T) {
	for id, info := range regions {
		if info.label == "" {
			t.Errorf("region %q has no label", id)
		}
		if info.flag == "" {
			t.Errorf("region %q has no flag", id)
		}
	}
}

func TestListClustersLabelsRegions(t *testing.T) {
	db, mock, _ := sqlmock.New()

	mock.ExpectQuery("SELECT ac.cluster_id, c.region, ac.is_default").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"cluster_id", "region", "is_default"}).
			AddRow("cluster-a", "eu-west-1", true))

	got, err := NewClusterBindings(db, clusterid.Resolver{}).List("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].RegionLabel != "Europe (Ireland)" || got[0].RegionFlag != "🇮🇪" {
		t.Fatalf("got %+v, want a labelled and flagged region", got)
	}
}
