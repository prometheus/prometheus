package tsdb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddJitterToChunkEndTime_ShouldHonorMaxVarianceAndMaxNextAt(t *testing.T) {
	chunkMinTime := int64(10)
	nextAt := int64(95)
	maxNextAt := int64(100)
	variance := 0.2

	// Compute the expected max variance.
	expectedMaxVariance := int64(float64(nextAt-chunkMinTime) * variance)

	for seriesHash := uint64(0); seriesHash < 1000; seriesHash++ {
		actual := addJitterToChunkEndTime(seriesHash, chunkMinTime, nextAt, maxNextAt, variance)
		require.GreaterOrEqual(t, actual, nextAt-(expectedMaxVariance/2))
		require.LessOrEqual(t, actual, maxNextAt)
	}
}

func TestAddJitterToChunkEndTime_Distribution(t *testing.T) {
	chunkMinTime := int64(0)
	nextAt := int64(50)
	maxNextAt := int64(100)
	variance := 0.2
	numSeries := uint64(1000)

	// Compute the expected max variance.
	expectedMaxVariance := int64(float64(nextAt-chunkMinTime) * variance)

	// Keep track of the distribution of the applied variance.
	varianceDistribution := map[int64]int64{}

	for seriesHash := uint64(0); seriesHash < numSeries; seriesHash++ {
		actual := addJitterToChunkEndTime(seriesHash, chunkMinTime, nextAt, maxNextAt, variance)
		require.GreaterOrEqual(t, actual, nextAt-(expectedMaxVariance/2))
		require.LessOrEqual(t, actual, nextAt+(expectedMaxVariance/2))
		require.LessOrEqual(t, actual, maxNextAt)

		variance := nextAt - actual
		varianceDistribution[variance]++
	}

	// Ensure a uniform distribution.
	for variance, count := range varianceDistribution {
		require.Equalf(t, int64(numSeries)/expectedMaxVariance, count, "variance = %d", variance)
	}
}

func TestAddJitterToChunkEndTime_ShouldNotApplyJitterToTheLastChunkOfTheRange(t *testing.T) {
	// Since the jitter could also be 0, we try it for multiple series.
	for seriesHash := uint64(0); seriesHash < 10; seriesHash++ {
		require.Equal(t, int64(200), addJitterToChunkEndTime(seriesHash, 150, 200, 200, 0.2))
	}
}

func TestAddJitterToChunkEndTime_ShouldNotApplyJitterIfDisabled(t *testing.T) {
	// Since the jitter could also be 0, we try it for multiple series.
	for seriesHash := uint64(0); seriesHash < 10; seriesHash++ {
		require.Equal(t, int64(130), addJitterToChunkEndTime(seriesHash, 100, 130, 200, 0))
	}
}
