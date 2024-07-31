package relabeler

import (
	"math"
	"time"

	"github.com/jonboulle/clockwork"
)

// Autosharder - selects a new value for the number of shards
// based on the achievement of time or block size limits.
type Autosharder struct {
	clock                    clockwork.Clock
	start                    time.Time
	cfg                      BlockLimits
	currentShardsNumberPower uint8
}

// NewAutosharder - init new Autosharder.
func NewAutosharder(
	clock clockwork.Clock,
	cfg BlockLimits,
	shardsNumberPower uint8,
) *Autosharder {
	return &Autosharder{
		clock:                    clock,
		start:                    clock.Now(),
		cfg:                      cfg,
		currentShardsNumberPower: shardsNumberPower,
	}
}

// ShardsNumberPower - calculate new shards number of power.
func (as *Autosharder) ShardsNumberPower(blockBytes int64) uint8 {
	switch {
	case blockBytes > as.cfg.DesiredBlockSizeBytes:
		as.currentShardsNumberPower = as.calculateByTime()
		return as.currentShardsNumberPower
	case as.clock.Since(as.start).Seconds() >= as.cfg.DesiredBlockFormationDuration.Seconds():
		//revive:disable-next-line:add-constant 100% not ned constant
		size := float64((blockBytes * (100 + as.cfg.BlockSizePercentThresholdForDownscaling)) / 100)
		as.currentShardsNumberPower = as.calculateBySize(size)
		return as.currentShardsNumberPower
	default:
		return as.currentShardsNumberPower
	}
}

// calculateByTime - calculate by elapsed time.
func (as *Autosharder) calculateByTime() uint8 {
	k := as.clock.Since(as.start).Seconds() / as.cfg.DesiredBlockFormationDuration.Seconds()
	//revive:disable:add-constant edge values is more readable without constants
	switch {
	case k > 1:
		return as.currentShardsNumberPower
	case k > 0.5:
		return as.incShardsNumberPower(1)
	case k > 0.25:
		return as.incShardsNumberPower(2)
	case k > 0.125:
		return as.incShardsNumberPower(3)
	case k > 0.0625:
		return as.incShardsNumberPower(4)
	default:
		return as.incShardsNumberPower(5)
	}
	//revive:enable
}

// calculateBySize - calculate by elapsed block size.
func (as *Autosharder) calculateBySize(blockBytes float64) uint8 {
	k := blockBytes / float64(as.cfg.DesiredBlockSizeBytes)
	//revive:disable:add-constant edge values is more readable without constants
	switch {
	case k > 0.5:
		return as.currentShardsNumberPower
	case k > 0.25:
		return as.decShardsNumberPower(1)
	case k > 0.125:
		return as.decShardsNumberPower(2)
	case k > 0.0625:
		return as.decShardsNumberPower(3)
	case k > 0.03125:
		return as.decShardsNumberPower(4)
	default:
		return as.decShardsNumberPower(5)
	}
	//revive:enable
}

// incShardsNumberPower - increase the current ShardsNumberPower considering the maximum value.
func (as *Autosharder) incShardsNumberPower(in uint8) uint8 {
	if as.currentShardsNumberPower >= math.MaxUint8-in {
		return math.MaxUint8
	}
	return as.currentShardsNumberPower + in
}

// decShardsNumberPower - decrease the current ShardsNumberPower considering the minimum value
func (as *Autosharder) decShardsNumberPower(in uint8) uint8 {
	if as.currentShardsNumberPower <= in {
		return 0
	}
	return as.currentShardsNumberPower - in
}

// Reset - reset state Autosharder.
func (as *Autosharder) Reset(cfg BlockLimits) {
	as.start = as.clock.Now()
	as.cfg = cfg
}
