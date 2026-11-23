package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestGroupEntityToServiceNormalizesHistoricalZeroFiveHourLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit float64
		want  *float64
	}{
		{name: "zero becomes nil", limit: 0, want: nil},
		{name: "negative becomes nil", limit: -1, want: nil},
		{name: "positive cent preserved", limit: 0.01, want: ptrRepoFloat(0.01)},
		{name: "positive amount preserved", limit: 2.5, want: ptrRepoFloat(2.5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupEntityToService(&dbent.Group{FiveHourLimitUsd: &tt.limit})
			require.NotNil(t, got)
			if tt.want == nil {
				require.Nil(t, got.FiveHourLimitUSD)
				return
			}
			require.NotNil(t, got.FiveHourLimitUSD)
			require.InDelta(t, *tt.want, *got.FiveHourLimitUSD, 1e-9)
		})
	}
}

func ptrRepoFloat(v float64) *float64 { return &v }
