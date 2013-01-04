package retrieval

import (
	"encoding/json"
	"github.com/matttproud/prometheus/model"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

type TargetPool []*Target

func (p TargetPool) Len() int {
	return len(p)
}

func (p TargetPool) Less(i, j int) bool {
	return p[i].scheduledFor.Before(p[j].scheduledFor)
}

func (p *TargetPool) Pop() interface{} {
	oldPool := *p
	futureLength := p.Len() - 1
	element := oldPool[futureLength]
	futurePool := oldPool[0:futureLength]
	*p = futurePool

	return element
}

func (p *TargetPool) Push(element interface{}) {
	*p = append(*p, element.(*Target))
}

func (p TargetPool) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}
