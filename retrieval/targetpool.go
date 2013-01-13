package retrieval

import (
	"container/heap"
	"log"
	"time"
)

type TargetPool struct {
	done    chan bool
	targets []*target
	manager TargetManager
}

func NewTargetPool(m TargetManager) (p TargetPool) {
	p.manager = m

	return
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
	p.targets = append(p.targets, element.(*target))
}

func (p TargetPool) Swap(i, j int) {
	p.targets[i], p.targets[j] = p.targets[j], p.targets[i]
}

func (p *TargetPool) Run(results chan Result, interval time.Duration) {
	ticker := time.Tick(interval)

	for {
		select {
		case <-ticker:
			p.runIteration(results)
		case <-p.done:
			log.Printf("TargetPool exiting...")
			break
		}
	}
}

func (p TargetPool) Stop() {
	p.done <- true
}

func (p *TargetPool) runSingle(earliest time.Time, results chan Result, t *target) {
	p.manager.acquire()
	defer p.manager.release()

	t.Scrape(earliest, results)
}

func (p *TargetPool) runIteration(results chan Result) {
	for i := 0; i < p.Len(); i++ {
		target := heap.Pop(p).(*target)
		if target == nil {
			break
		}

		now := time.Now()

		if target.scheduledFor().After(now) {
			heap.Push(p, target)

			break
		}

		go func() {
			p.runSingle(now, results, target)
			heap.Push(p, target)
		}()
	}
}
