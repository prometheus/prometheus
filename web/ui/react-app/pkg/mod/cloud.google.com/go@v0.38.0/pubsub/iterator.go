// Copyright 2016 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pubsub

import (
	"context"
	"io"
	"sync"
	"time"

	vkit "cloud.google.com/go/pubsub/apiv1"
	"cloud.google.com/go/pubsub/internal/distribution"
	gax "github.com/googleapis/gax-go/v2"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Between message receipt and ack (that is, the time spent processing a message) we want to extend the message
// deadline by way of modack. However, we don't want to extend the deadline right as soon as the deadline expires;
// instead, we'd want to extend the deadline a little bit of time ahead. gracePeriod is that amount of time ahead
// of the actual deadline.
const gracePeriod = 5 * time.Second

type messageIterator struct {
	ctx        context.Context
	cancel     func() // the function that will cancel ctx; called in stop
	po         *pullOptions
	ps         *pullStream
	subc       *vkit.SubscriberClient
	subName    string
	kaTick     <-chan time.Time // keep-alive (deadline extensions)
	ackTicker  *time.Ticker     // message acks
	nackTicker *time.Ticker     // message nacks (more frequent than acks)
	pingTicker *time.Ticker     //  sends to the stream to keep it open
	failed     chan struct{}    // closed on stream error
	drained    chan struct{}    // closed when stopped && no more pending messages
	wg         sync.WaitGroup

	mu          sync.Mutex
	ackTimeDist *distribution.D // dist uses seconds

	// keepAliveDeadlines is a map of id to expiration time. This map is used in conjunction with
	// subscription.ReceiveSettings.MaxExtension to record the maximum amount of time (the
	// deadline, more specifically) we're willing to extend a message's ack deadline. As each
	// message arrives, we'll record now+MaxExtension in this table; whenever we have a chance
	// to update ack deadlines (via modack), we'll consult this table and only include IDs
	// that are not beyond their deadline.
	keepAliveDeadlines map[string]time.Time
	pendingAcks        map[string]bool
	pendingNacks       map[string]bool
	pendingModAcks     map[string]bool // ack IDs whose ack deadline is to be modified
	err                error           // error from stream failure
}

// newMessageIterator starts and returns a new messageIterator.
// subName is the full name of the subscription to pull messages from.
// Stop must be called on the messageIterator when it is no longer needed.
// The iterator always uses the background context for acking messages and extending message deadlines.
func newMessageIterator(subc *vkit.SubscriberClient, subName string, po *pullOptions) *messageIterator {
	var ps *pullStream
	if !po.synchronous {
		ps = newPullStream(context.Background(), subc.StreamingPull, subName)
	}
	// The period will update each tick based on the distribution of acks. We'll start by arbitrarily sending
	// the first keepAlive halfway towards the minimum ack deadline.
	keepAlivePeriod := minAckDeadline / 2

	// Ack promptly so users don't lose work if client crashes.
	ackTicker := time.NewTicker(100 * time.Millisecond)
	nackTicker := time.NewTicker(100 * time.Millisecond)
	pingTicker := time.NewTicker(30 * time.Second)
	cctx, cancel := context.WithCancel(context.Background())
	it := &messageIterator{
		ctx:                cctx,
		cancel:             cancel,
		ps:                 ps,
		po:                 po,
		subc:               subc,
		subName:            subName,
		kaTick:             time.After(keepAlivePeriod),
		ackTicker:          ackTicker,
		nackTicker:         nackTicker,
		pingTicker:         pingTicker,
		failed:             make(chan struct{}),
		drained:            make(chan struct{}),
		ackTimeDist:        distribution.New(int(maxAckDeadline/time.Second) + 1),
		keepAliveDeadlines: map[string]time.Time{},
		pendingAcks:        map[string]bool{},
		pendingNacks:       map[string]bool{},
		pendingModAcks:     map[string]bool{},
	}
	it.wg.Add(1)
	go it.sender()
	return it
}

// Subscription.receive will call stop on its messageIterator when finished with it.
// Stop will block until Done has been called on all Messages that have been
// returned by Next, or until the context with which the messageIterator was created
// is cancelled or exceeds its deadline.
func (it *messageIterator) stop() {
	it.cancel()
	it.mu.Lock()
	it.checkDrained()
	it.mu.Unlock()
	it.wg.Wait()
}

// checkDrained closes the drained channel if the iterator has been stopped and all
// pending messages have either been n/acked or expired.
//
// Called with the lock held.
func (it *messageIterator) checkDrained() {
	select {
	case <-it.drained:
		return
	default:
	}
	select {
	case <-it.ctx.Done():
		if len(it.keepAliveDeadlines) == 0 {
			close(it.drained)
		}
	default:
	}
}

// Called when a message is acked/nacked.
func (it *messageIterator) done(ackID string, ack bool, receiveTime time.Time) {
	it.ackTimeDist.Record(int(time.Since(receiveTime) / time.Second))
	it.mu.Lock()
	defer it.mu.Unlock()
	delete(it.keepAliveDeadlines, ackID)
	if ack {
		it.pendingAcks[ackID] = true
	} else {
		it.pendingNacks[ackID] = true
	}
	it.checkDrained()
}

// fail is called when a stream method returns a permanent error.
// fail returns it.err. This may be err, or it may be the error
// set by an earlier call to fail.
func (it *messageIterator) fail(err error) error {
	it.mu.Lock()
	defer it.mu.Unlock()
	if it.err == nil {
		it.err = err
		close(it.failed)
	}
	return it.err
}

// receive makes a call to the stream's Recv method, or the Pull RPC, and returns
// its messages.
// maxToPull is the maximum number of messages for the Pull RPC.
func (it *messageIterator) receive(maxToPull int32) ([]*Message, error) {
	it.mu.Lock()
	ierr := it.err
	it.mu.Unlock()
	if ierr != nil {
		return nil, ierr
	}

	// Stop retrieving messages if the iterator's Stop method was called.
	select {
	case <-it.ctx.Done():
		it.wg.Wait()
		return nil, io.EOF
	default:
	}

	var rmsgs []*pb.ReceivedMessage
	var err error
	if it.po.synchronous {
		rmsgs, err = it.pullMessages(maxToPull)
	} else {
		rmsgs, err = it.recvMessages()
	}
	// Any error here is fatal.
	if err != nil {
		return nil, it.fail(err)
	}
	msgs, err := convertMessages(rmsgs)
	if err != nil {
		return nil, it.fail(err)
	}
	// We received some messages. Remember them so we can keep them alive. Also,
	// do a receipt mod-ack when streaming.
	maxExt := time.Now().Add(it.po.maxExtension)
	ackIDs := map[string]bool{}
	it.mu.Lock()
	now := time.Now()
	for _, m := range msgs {
		m.receiveTime = now
		addRecv(m.ID, m.ackID, now)
		m.doneFunc = it.done
		it.keepAliveDeadlines[m.ackID] = maxExt
		// Don't change the mod-ack if the message is going to be nacked. This is
		// possible if there are retries.
		if !it.pendingNacks[m.ackID] {
			ackIDs[m.ackID] = true
		}
	}
	deadline := it.ackDeadline()
	it.mu.Unlock()
	if len(ackIDs) > 0 {
		if !it.sendModAck(ackIDs, deadline) {
			return nil, it.err
		}
	}
	return msgs, nil
}

// Get messages using the Pull RPC.
// This may block indefinitely. It may also return zero messages, after some time waiting.
func (it *messageIterator) pullMessages(maxToPull int32) ([]*pb.ReceivedMessage, error) {
	// Use it.ctx as the RPC context, so that if the iterator is stopped, the call
	// will return immediately.
	res, err := it.subc.Pull(it.ctx, &pb.PullRequest{
		Subscription: it.subName,
		MaxMessages:  maxToPull,
	}, gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(maxSendRecvBytes)))
	switch {
	case err == context.Canceled:
		return nil, nil
	case err != nil:
		return nil, err
	default:
		return res.ReceivedMessages, nil
	}
}

func (it *messageIterator) recvMessages() ([]*pb.ReceivedMessage, error) {
	res, err := it.ps.Recv()
	if err != nil {
		return nil, err
	}
	return res.ReceivedMessages, nil
}

// sender runs in a goroutine and handles all sends to the stream.
func (it *messageIterator) sender() {
	defer it.wg.Done()
	defer it.ackTicker.Stop()
	defer it.nackTicker.Stop()
	defer it.pingTicker.Stop()
	defer func() {
		if it.ps != nil {
			it.ps.CloseSend()
		}
	}()

	done := false
	for !done {
		sendAcks := false
		sendNacks := false
		sendModAcks := false
		sendPing := false

		dl := it.ackDeadline()

		select {
		case <-it.failed:
			// Stream failed: nothing to do, so stop immediately.
			return

		case <-it.drained:
			// All outstanding messages have been marked done:
			// nothing left to do except make the final calls.
			it.mu.Lock()
			sendAcks = (len(it.pendingAcks) > 0)
			sendNacks = (len(it.pendingNacks) > 0)
			// No point in sending modacks.
			done = true

		case <-it.kaTick:
			it.mu.Lock()
			it.handleKeepAlives()
			sendModAcks = (len(it.pendingModAcks) > 0)

			nextTick := dl - gracePeriod
			if nextTick <= 0 {
				// If the deadline is <= gracePeriod, let's tick again halfway to
				// the deadline.
				nextTick = dl / 2
			}
			it.kaTick = time.After(nextTick)

		case <-it.nackTicker.C:
			it.mu.Lock()
			sendNacks = (len(it.pendingNacks) > 0)

		case <-it.ackTicker.C:
			it.mu.Lock()
			sendAcks = (len(it.pendingAcks) > 0)

		case <-it.pingTicker.C:
			it.mu.Lock()
			// Ping only if we are processing messages via streaming.
			sendPing = !it.po.synchronous && (len(it.keepAliveDeadlines) > 0)
		}
		// Lock is held here.
		var acks, nacks, modAcks map[string]bool
		if sendAcks {
			acks = it.pendingAcks
			it.pendingAcks = map[string]bool{}
		}
		if sendNacks {
			nacks = it.pendingNacks
			it.pendingNacks = map[string]bool{}
		}
		if sendModAcks {
			modAcks = it.pendingModAcks
			it.pendingModAcks = map[string]bool{}
		}
		it.mu.Unlock()
		// Make Ack and ModAck RPCs.
		if sendAcks {
			if !it.sendAck(acks) {
				return
			}
		}
		if sendNacks {
			// Nack indicated by modifying the deadline to zero.
			if !it.sendModAck(nacks, 0) {
				return
			}
		}
		if sendModAcks {
			if !it.sendModAck(modAcks, dl) {
				return
			}
		}
		if sendPing {
			it.pingStream()
		}
	}
}

// handleKeepAlives modifies the pending request to include deadline extensions
// for live messages. It also purges expired messages.
//
// Called with the lock held.
func (it *messageIterator) handleKeepAlives() {
	now := time.Now()
	for id, expiry := range it.keepAliveDeadlines {
		if expiry.Before(now) {
			// This delete will not result in skipping any map items, as implied by
			// the spec at https://golang.org/ref/spec#For_statements, "For
			// statements with range clause", note 3, and stated explicitly at
			// https://groups.google.com/forum/#!msg/golang-nuts/UciASUb03Js/pzSq5iVFAQAJ.
			delete(it.keepAliveDeadlines, id)
		} else {
			// This will not conflict with a nack, because nacking removes the ID from keepAliveDeadlines.
			it.pendingModAcks[id] = true
		}
	}
	it.checkDrained()
}

func (it *messageIterator) sendAck(m map[string]bool) bool {
	return it.sendAckIDRPC(m, func(ids []string) error {
		recordStat(it.ctx, AckCount, int64(len(ids)))
		addAcks(ids)
		// Use context.Background() as the call's context, not it.ctx. We don't
		// want to cancel this RPC when the iterator is stopped.
		return it.subc.Acknowledge(context.Background(), &pb.AcknowledgeRequest{
			Subscription: it.subName,
			AckIds:       ids,
		})
	})
}

// The receipt mod-ack amount is derived from a percentile distribution based
// on the time it takes to process messages. The percentile chosen is the 99%th
// percentile in order to capture the highest amount of time necessary without
// considering 1% outliers.
func (it *messageIterator) sendModAck(m map[string]bool, deadline time.Duration) bool {
	return it.sendAckIDRPC(m, func(ids []string) error {
		if deadline == 0 {
			recordStat(it.ctx, NackCount, int64(len(ids)))
		} else {
			recordStat(it.ctx, ModAckCount, int64(len(ids)))
		}
		addModAcks(ids, int32(deadline/time.Second))
		// Retry this RPC on Unavailable for a short amount of time, then give up
		// without returning a fatal error. The utility of this RPC is by nature
		// transient (since the deadline is relative to the current time) and it
		// isn't crucial for correctness (since expired messages will just be
		// resent).
		cctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		bo := gax.Backoff{
			Initial:    100 * time.Millisecond,
			Max:        time.Second,
			Multiplier: 2,
		}
		for {
			err := it.subc.ModifyAckDeadline(cctx, &pb.ModifyAckDeadlineRequest{
				Subscription:       it.subName,
				AckDeadlineSeconds: int32(deadline / time.Second),
				AckIds:             ids,
			})
			switch status.Code(err) {
			case codes.Unavailable:
				if err := gax.Sleep(cctx, bo.Pause()); err == nil {
					continue
				}
				// Treat sleep timeout like RPC timeout.
				fallthrough
			case codes.DeadlineExceeded:
				// Timeout. Not a fatal error, but note that it happened.
				recordStat(it.ctx, ModAckTimeoutCount, 1)
				return nil
			default:
				// Any other error is fatal.
				return err
			}
		}
	})
}

func (it *messageIterator) sendAckIDRPC(ackIDSet map[string]bool, call func([]string) error) bool {
	ackIDs := make([]string, 0, len(ackIDSet))
	for k := range ackIDSet {
		ackIDs = append(ackIDs, k)
	}
	var toSend []string
	for len(ackIDs) > 0 {
		toSend, ackIDs = splitRequestIDs(ackIDs, maxPayload)
		if err := call(toSend); err != nil {
			// The underlying client handles retries, so any error is fatal to the
			// iterator.
			it.fail(err)
			return false
		}
	}
	return true
}

// Send a message to the stream to keep it open. The stream will close if there's no
// traffic on it for a while. By keeping it open, we delay the start of the
// expiration timer on messages that are buffered by gRPC or elsewhere in the
// network. This matters if it takes a long time to process messages relative to the
// default ack deadline, and if the messages are small enough so that many can fit
// into the buffer.
func (it *messageIterator) pingStream() {
	// Ignore error; if the stream is broken, this doesn't matter anyway.
	_ = it.ps.Send(&pb.StreamingPullRequest{})
}

func splitRequestIDs(ids []string, maxSize int) (prefix, remainder []string) {
	size := reqFixedOverhead
	i := 0
	for size < maxSize && i < len(ids) {
		size += overheadPerID + len(ids[i])
		i++
	}
	if size > maxSize {
		i--
	}
	return ids[:i], ids[i:]
}

// The deadline to ack is derived from a percentile distribution based
// on the time it takes to process messages. The percentile chosen is the 99%th
// percentile - that is, processing times up to the 99%th longest processing
// times should be safe. The highest 1% may expire. This number was chosen
// as a way to cover most users' usecases without losing the value of
// expiration.
func (it *messageIterator) ackDeadline() time.Duration {
	pt := time.Duration(it.ackTimeDist.Percentile(.99)) * time.Second

	if pt > maxAckDeadline {
		return maxAckDeadline
	}
	if pt < minAckDeadline {
		return minAckDeadline
	}
	return pt
}
