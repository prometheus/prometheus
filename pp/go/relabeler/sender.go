package relabeler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

// defaultRTT - default value RTT.
const (
	defaultRTT = 120 * time.Second
)

// Source is a manager
type Source interface {
	Get(ctx context.Context, key cppbridge.SegmentKey) (Segment, error)
	Ack(key cppbridge.SegmentKey, dest string)
	Reject(key cppbridge.SegmentKey, dest string)
}

// Sender is a transport adapter for manager
type Sender struct {
	done          chan struct{}
	dialer        Dialer
	source        Source
	cancelCause   context.CancelCauseFunc
	readAt        *atomic.Int64
	writeAt       *atomic.Int64
	hasRejects    *atomic.Bool
	shardMeta     ShardMeta
	lastDelivered uint32
	// stat
	exceededRTT      prometheus.Counter
	sentSegment      prometheus.Gauge
	responsedSegment *prometheus.GaugeVec
}

// NewSender is a constructor
func NewSender(
	ctx context.Context,
	blockID uuid.UUID,
	shardID uint16,
	shardsLog, segmentEncodingVersion uint8,
	dialer Dialer,
	lastAck uint32,
	source Source,
	name string,
	registerer prometheus.Registerer,
) *Sender {
	factory := util.NewUnconflictRegisterer(registerer)
	sender := &Sender{
		dialer: dialer,
		source: source,
		shardMeta: ShardMeta{
			BlockID:                blockID,
			ShardID:                shardID,
			ShardsLog:              shardsLog,
			SegmentEncodingVersion: segmentEncodingVersion,
		},
		lastDelivered: lastAck,
		done:          make(chan struct{}),
		readAt:        new(atomic.Int64),
		writeAt:       new(atomic.Int64),
		hasRejects:    new(atomic.Bool),
		sentSegment: factory.NewGauge(
			prometheus.GaugeOpts{
				Name:        "prompp_delivery_sender_sent_segment",
				Help:        "Sent segment ID.",
				ConstLabels: prometheus.Labels{"host": dialer.String(), "name": name},
			},
		),
		responsedSegment: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name:        "prompp_delivery_sender_responsed_segment",
				Help:        "Responsed segment ID.",
				ConstLabels: prometheus.Labels{"host": dialer.String(), "name": name},
			},
			[]string{"state"},
		),
		exceededRTT: factory.NewCounter(
			prometheus.CounterOpts{
				Name:        "prompp_delivery_sender_exceeded_rtt",
				Help:        "Number of reconect on exceeded rtt.",
				ConstLabels: prometheus.Labels{"host": dialer.String(), "name": name},
			},
		),
	}
	sender.sentSegment.Set(0)
	sender.responsedSegment.With(prometheus.Labels{"state": "ack"}).Set(0)
	sender.responsedSegment.With(prometheus.Labels{"state": "reject"}).Set(0)
	ctx, cancel := context.WithCancelCause(ctx)
	sender.cancelCause = cancel
	go sender.mainLoop(ctx)
	return sender
}

// String implements fmt.Stringer interface
func (sender *Sender) String() string {
	return sender.dialer.String()
}

// Shutdown await while write receive ErrPromiseCanceled and then ack on last sent
func (sender *Sender) Shutdown(ctx context.Context) error {
	sender.cancelCause(ErrShutdown)

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-sender.done:
		return nil
	}
}

func (sender *Sender) dial(ctx context.Context) (transport Transport, closeFn func(), err error) {
	transport, err = sender.dialer.Dial(ctx, sender.shardMeta)
	if err != nil {
		if !errors.Is(err, ErrShutdown) && !errors.Is(err, context.Canceled) {
			Errorf("%s: fail to dial: %s", sender, err)
		}
		return nil, nil, err
	}

	closeFn = func() {
		if err := transport.Close(); err != nil {
			Errorf("%s: fail to close transport: %s", sender, err)
		}
	}

	return transport, closeFn, nil
}

// getSegment returns segment by sender shardID and given segment id
//
// It retry get segment from source with exponential backoff until one of next happened:
// - get segment without error (returns (segment, nil))
// - context canceled (returns (nil, context.Cause(ctx)))
// - get ErrPromiseCanceled (returns (nil, nil))
//
// So, it's correct to check that segment is nil, it is equivalent permanent state.
func (sender *Sender) getSegment(ctx context.Context, id uint32) (Segment, error) {
	key := cppbridge.SegmentKey{
		ShardID: sender.shardMeta.ShardID,
		Segment: id,
	}
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry until context cancel
	bo := backoff.WithContext(eb, ctx)
	segment, err := backoff.RetryWithData(func() (Segment, error) {
		segment, err := sender.source.Get(ctx, key)
		if errors.Is(err, ErrPromiseCanceled) {
			return nil, backoff.Permanent(ErrBlockFinalization)
		}
		return segment, err
	}, bo)
	if err != nil && err == ctx.Err() {
		err = context.Cause(ctx)
	}
	return segment, err
}

//revive:disable-next-line:cognitive-complexity function is not complicated
//revive:disable-next-line:function-length long but readable
func (sender *Sender) mainLoop(ctx context.Context) {
	writeDone := new(atomic.Bool)
	errRead := errors.New("read error")
	for ctx.Err() == nil {
		transport, closeTransport, err := sender.dial(ctx)
		if err != nil {
			continue
		}
		lastSent := sender.lastDelivered
		onResponse := func(id uint32) {
			sender.readAt.Store(time.Now().UnixMilli())
			if !atomic.CompareAndSwapUint32(&sender.lastDelivered, id-1, id) {
				panic(fmt.Sprintf("%s: unexpected segment %d (lastDelivered %d)", sender, id, sender.lastDelivered))
			}
			if writeDone.Load() && lastSent == id {
				closeTransport()
				close(sender.done)
			}
		}
		transport.OnAck(func(id uint32) {
			sender.responsedSegment.With(prometheus.Labels{"state": "ack"}).Set(float64(id))
			sender.source.Ack(cppbridge.SegmentKey{ShardID: sender.shardMeta.ShardID, Segment: id}, sender.String())
			onResponse(id)
		})
		transport.OnReject(func(id uint32) {
			sender.hasRejects.Store(true)
			sender.responsedSegment.With(prometheus.Labels{"state": "reject"}).Set(float64(id))
			sender.source.Reject(cppbridge.SegmentKey{ShardID: sender.shardMeta.ShardID, Segment: id}, sender.String())
			onResponse(id)
		})
		writeCtx, cancel := context.WithCancelCause(ctx)
		transport.OnReadError(func(err error) {
			if !errors.Is(err, io.EOF) {
				Errorf("%s: fail to read response: %s", sender, err)
			}
			cancel(errRead)
		})
		sender.readAt.Store(time.Now().UnixMilli())
		transport.Listen(ctx)
		lastSent, err = sender.writeLoop(writeCtx, transport, lastSent)
		if err != nil {
			if !errors.Is(err, errRead) {
				Errorf("%s: fail to send segment: %s", sender, err)
			}
			closeTransport()
			continue
		}
		// transport will be closed by reader otherwise
		if lastSent == atomic.LoadUint32(&sender.lastDelivered) {
			closeTransport()
			close(sender.done)
		}
		writeDone.Store(true)
		return
	}
	close(sender.done)
}

func (sender *Sender) writeLoop(ctx context.Context, transport Transport, from uint32) (uint32, error) {
	id := from + 1
	for ; ctx.Err() == nil; id++ {
		segment, err := sender.getSegment(ctx, id)
		if err != nil && errors.Is(err, ErrBlockFinalization) {
			if sender.hasRejects.Load() {
				return id - 1, nil
			}
			ctxt, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			errFinal := transport.Send(ctxt, frames.NewWriteSegmentV4(id, nil))
			cancel()
			return id - 1, errFinal
		}
		if segment == nil {
			return id - 1, err
		}
		if sender.writeAt.Load()-sender.readAt.Load() > defaultRTT.Milliseconds() {
			sender.exceededRTT.Inc()
			return id - 1, ErrExceededRTT
		}

		if err = transport.Send(ctx, frames.NewWriteSegmentV4(id, segment)); err != nil {
			return id - 1, err
		}
		sender.writeAt.Store(time.Now().UnixMilli())
		sender.sentSegment.Set(float64(id))
	}
	return id - 1, context.Cause(ctx)
}
