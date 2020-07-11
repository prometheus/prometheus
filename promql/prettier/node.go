package prettier

import (
	"github.com/prometheus/prometheus/promql/parser"
)

type nodeInfo struct {
	node parser.Expr
	// node information details.
	// information details should be used only after calling the fetch.
	shouldSplit bool
	columnLimit int
}

func newNodeInfo(node parser.Expr, columnLimit int) *nodeInfo {
	return &nodeInfo{
		node:        node,
		columnLimit: columnLimit,
	}
}

// fetch fetches the node information.
func (n *nodeInfo) fetch() {
	n.shouldSplit = n.violatesColumnLimit()
}

func (n *nodeInfo) violatesColumnLimit() bool {
	return n.getNodeSize() > n.columnLimit
}

func (n *nodeInfo) getNodeSize() int {
	return len(n.node.String())
}
