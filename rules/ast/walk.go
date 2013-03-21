// Copyright 2013 Prometheus Team
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

type Visitor interface {
	Visit(node Node)
}

// Walk() does a depth-first traversal of the AST, calling visitor.Visit() for
// each encountered node in the tree.
func Walk(visitor Visitor, node Node) {
	visitor.Visit(node)
	for _, childNode := range node.Children() {
		Walk(visitor, childNode)
	}
}
