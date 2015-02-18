// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ast

// visitor is the interface for a Node visitor.
type visitor interface {
	visit(node Node)
}

// Walk does a depth-first traversal of the AST, starting at node,
// calling visitor.visit for each encountered Node in the tree.
func Walk(v visitor, node Node) {
	v.visit(node)
	for _, childNode := range node.Children() {
		Walk(v, childNode)
	}
}
