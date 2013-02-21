package retrieval

import (
	"container/heap"
	"github.com/prometheus/prometheus/retrieval/format"
	"log"
	"time"
)

const (
	intervalKey = "interval"
)

type TargetPool struct {
	done    chan bool
	manager TargetManager
	targets []Target
}

func NewTargetPool(m TargetManager) (p *TargetPool) {
	return &TargetPool{
		manager: m,
	}
}

func (p TargetPool) Len() int {
	return len(p.targets)
}

func (p TargetPool) Less(i, j int) bool {
	return p.targets[i].scheduledFor().Before(p.targets[j].scheduledFor())
}

func (p *TargetPool) Pop() interface{} {
	oldPool := p.targets
	futureLength := p.Len() - 1
	element := oldPool[futureLength]
	futurePool := oldPool[0:futureLength]
	p.targets = futurePool

	return element
}

func (p *TargetPool) Push(element interface{}) {
	p.targets = append(p.targets, element.(Target))
}

func (p TargetPool) Swap(i, j int) {
	p.targets[i], p.targets[j] = p.targets[j], p.targets[i]
}

func (p *TargetPool) Run(results chan format.Result, interval time.Duration) {
	ticker := time.Tick(interval)

	for {
		select {
		case <-ticker:
			p.runIteration(results, interval)
		case <-p.done:
			log.Printf("TargetPool exiting...")
			break
		}
	}
}

func (p TargetPool) Stop() {
	p.done <- true
}

func (p *TargetPool) runSingle(earliest time.Time, results chan format.Result, t Target) {
	p.manager.acquire()
	defer p.manager.release()

	t.Scrape(earliest, results)
}

func (p *TargetPool) runIteration(results chan format.Result, interval time.Duration) {
	begin := time.Now()

	targetCount := p.Len()
	finished := make(chan bool, targetCount)

	for i := 0; i < targetCount; i++ {
		target := heap.Pop(p).(Target)
		if target == nil {
			break
		}

		now := time.Now()

		if target.scheduledFor().After(now) {
			heap.Push(p, target)
			// None of the remaining targets are ready to be scheduled. Signal that
			// we're done processing them in this scrape iteration.
			for j := i; j < targetCount; j++ {
				finished <- true
			}
			break
		}

		go func() {
			p.runSingle(now, results, target)
			heap.Push(p, target)
			finished <- true
		}()
	}

	for i := 0; i < targetCount; i++ {
		<-finished
	}

	close(finished)

	duration := float64(time.Since(begin) / time.Millisecond)
	retrievalDurations.Add(map[string]string{intervalKey: interval.String()}, duration)
}
