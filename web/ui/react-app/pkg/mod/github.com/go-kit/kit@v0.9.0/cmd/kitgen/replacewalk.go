package main

import (
	"fmt"
	"go/ast"
)

// A Visitor's Visit method is invoked for each node encountered by walkToReplace.
// If the result visitor w is not nil, walkToReplace visits each of the children
// of node with the visitor w, followed by a call of w.Visit(nil).
type Visitor interface {
	Visit(node ast.Node, replace func(ast.Node)) (w Visitor)
}

// Helper functions for common node lists. They may be empty.

func walkIdentList(v Visitor, list []*ast.Ident) {
	for i, x := range list {
		walkToReplace(v, x, func(r ast.Node) {
			list[i] = r.(*ast.Ident)
		})
	}
}

func walkExprList(v Visitor, list []ast.Expr) {
	for i, x := range list {
		walkToReplace(v, x, func(r ast.Node) {
			list[i] = r.(ast.Expr)
		})
	}
}

func walkStmtList(v Visitor, list []ast.Stmt) {
	for i, x := range list {
		walkToReplace(v, x, func(r ast.Node) {
			list[i] = r.(ast.Stmt)
		})
	}
}

func walkDeclList(v Visitor, list []ast.Decl) {
	for i, x := range list {
		walkToReplace(v, x, func(r ast.Node) {
			list[i] = r.(ast.Decl)
		})
	}
}

// WalkToReplace traverses an AST in depth-first order: It starts by calling
// v.Visit(node); node must not be nil. If the visitor w returned by
// v.Visit(node) is not nil, walkToReplace is invoked recursively with visitor
// w for each of the non-nil children of node, followed by a call of
// w.Visit(nil).
func WalkReplace(v Visitor, node ast.Node) (replacement ast.Node) {
	walkToReplace(v, node, func(r ast.Node) {
		replacement = r
	})
	return
}

func walkToReplace(v Visitor, node ast.Node, replace func(ast.Node)) {
	if v == nil {
		return
	}
	var replacement ast.Node
	repl := func(r ast.Node) {
		replacement = r
		replace(r)
	}

	v = v.Visit(node, repl)

	if replacement != nil {
		return
	}

	// walk children
	// (the order of the cases matches the order
	// of the corresponding node types in ast.go)
	switch n := node.(type) {

	// These are all leaves, so there's no sub-walk to do.
	// We just need to replace them on their parent with a copy.
	case *ast.Comment:
		cpy := *n
		replace(&cpy)
	case *ast.BadExpr:
		cpy := *n
		replace(&cpy)
	case *ast.Ident:
		cpy := *n
		replace(&cpy)
	case *ast.BasicLit:
		cpy := *n
		replace(&cpy)
	case *ast.BadDecl:
		cpy := *n
		replace(&cpy)
	case *ast.EmptyStmt:
		cpy := *n
		replace(&cpy)
	case *ast.BadStmt:
		cpy := *n
		replace(&cpy)

	case *ast.CommentGroup:
		cpy := *n

		if n.List != nil {
			cpy.List = make([]*ast.Comment, len(n.List))
			copy(cpy.List, n.List)
		}

		for i, c := range cpy.List {
			walkToReplace(v, c, func(r ast.Node) {
				cpy.List[i] = r.(*ast.Comment)
			})
		}
		replace(&cpy)

	case *ast.Field:
		cpy := *n
		if n.Names != nil {
			cpy.Names = make([]*ast.Ident, len(n.Names))
			copy(cpy.Names, n.Names)
		}

		if cpy.Doc != nil {
			walkToReplace(v, cpy.Doc, func(r ast.Node) {
				cpy.Doc = r.(*ast.CommentGroup)
			})
		}
		walkIdentList(v, cpy.Names)

		walkToReplace(v, cpy.Type, func(r ast.Node) {
			cpy.Type = r.(ast.Expr)
		})
		if cpy.Tag != nil {
			walkToReplace(v, cpy.Tag, func(r ast.Node) {
				cpy.Tag = r.(*ast.BasicLit)
			})
		}
		if cpy.Comment != nil {
			walkToReplace(v, cpy.Comment, func(r ast.Node) {
				cpy.Comment = r.(*ast.CommentGroup)
			})
		}
		replace(&cpy)

	case *ast.FieldList:
		cpy := *n
		if n.List != nil {
			cpy.List = make([]*ast.Field, len(n.List))
			copy(cpy.List, n.List)
		}

		for i, f := range cpy.List {
			walkToReplace(v, f, func(r ast.Node) {
				cpy.List[i] = r.(*ast.Field)
			})
		}

		replace(&cpy)

	case *ast.Ellipsis:
		cpy := *n

		if cpy.Elt != nil {
			walkToReplace(v, cpy.Elt, func(r ast.Node) {
				cpy.Elt = r.(ast.Expr)
			})
		}

		replace(&cpy)

	case *ast.FuncLit:
		cpy := *n
		walkToReplace(v, cpy.Type, func(r ast.Node) {
			cpy.Type = r.(*ast.FuncType)
		})
		walkToReplace(v, cpy.Body, func(r ast.Node) {
			cpy.Body = r.(*ast.BlockStmt)
		})

		replace(&cpy)
	case *ast.CompositeLit:
		cpy := *n
		if n.Elts != nil {
			cpy.Elts = make([]ast.Expr, len(n.Elts))
			copy(cpy.Elts, n.Elts)
		}

		if cpy.Type != nil {
			walkToReplace(v, cpy.Type, func(r ast.Node) {
				cpy.Type = r.(ast.Expr)
			})
		}
		walkExprList(v, cpy.Elts)

		replace(&cpy)
	case *ast.ParenExpr:
		cpy := *n
		walkToReplace(v, cpy.X, func(r ast.Node) {
			cpy.X = r.(ast.Expr)
		})

		replace(&cpy)
	case *ast.SelectorExpr:
		cpy := *n
		walkToReplace(v, cpy.X, func(r ast.Node) {
			cpy.X = r.(ast.Expr)
		})
		walkToReplace(v, cpy.Sel, func(r ast.Node) {
			cpy.Sel = r.(*ast.Ident)
		})

		replace(&cpy)
	case *ast.IndexExpr:
		cpy := *n
		walkToReplace(v, cpy.X, func(r ast.Node) {
			cpy.X = r.(ast.Expr)
		})
		walkToReplace(v, cpy.Index, func(r ast.Node) {
			cpy.Index = r.(ast.Expr)
		})

		replace(&cpy)
	case *ast.SliceExpr:
		cpy := *n
		walkToReplace(v, cpy.X, func(r ast.Node) {
			cpy.X = r.(ast.Expr)
		})
		if cpy.Low != nil {
			walkToReplace(v, cpy.Low, func(r ast.Node) {
				cpy.Low = r.(ast.Expr)
			})
		}
		if cpy.High != nil {
			walkToReplace(v, cpy.High, func(r ast.Node) {
				cpy.High = r.(ast.Expr)
			})
		}
		if cpy.Max != nil {
			walkToReplace(v, cpy.Max, func(r ast.Node) {
				cpy.Max = r.(ast.Expr)
			})
		}

		replace(&cpy)
	case *ast.TypeAssertExpr:
		cpy := *n
		walkToReplace(v, cpy.X, func(r ast.Node) {
			cpy.X = r.(ast.Expr)
		})
		if cpy.Type != nil {
			walkToReplace(v, cpy.Type, func(r ast.Node) {
				cpy.Type = r.(ast.Expr)
			})
		}
		replace(&cpy)
	case *ast.CallExpr:
		cpy := *n
		if n.Args != nil {
			cpy.Args = make([]ast.Expr, len(n.Args))
			copy(cpy.Args, n.Args)
		}

		walkToReplace(v, cpy.Fun, func(r ast.Node) {
			cpy.Fun = r.(ast.Expr)
		})
		walkExprList(v, cpy.Args)

		replace(&cpy)
	case *ast.StarExpr:
		cpy := *n
		walkToReplace(v, cpy.X, func(r ast.Node) {
			cpy.X = r.(ast.Expr)
		})

		replace(&cpy)
	case *ast.UnaryExpr:
		cpy := *n
		walkToReplace(v, cpy.X, func(r ast.Node) {
			cpy.X = r.(ast.Expr)
		})

		replace(&cpy)
	case *ast.BinaryExpr:
		cpy := *n
		walkToReplace(v, cpy.X, func(r ast.Node) {
			cpy.X = r.(ast.Expr)
		})
		walkToReplace(v, cpy.Y, func(r ast.Node) {
			cpy.Y = r.(ast.Expr)
		})

		replace(&cpy)
	case *ast.KeyValueExpr:
		cpy := *n
		walkToReplace(v, cpy.Key, func(r ast.Node) {
			cpy.Key = r.(ast.Expr)
		})
		walkToReplace(v, cpy.Value, func(r ast.Node) {
			cpy.Value = r.(ast.Expr)
		})

		replace(&cpy)

		// Types
	case *ast.ArrayType:
		cpy := *n
		if cpy.Len != nil {
			walkToReplace(v, cpy.Len, func(r ast.Node) {
				cpy.Len = r.(ast.Expr)
			})
		}
		walkToReplace(v, cpy.Elt, func(r ast.Node) {
			cpy.Elt = r.(ast.Expr)
		})

		replace(&cpy)
	case *ast.StructType:
		cpy := *n
		walkToReplace(v, cpy.Fields, func(r ast.Node) {
			cpy.Fields = r.(*ast.FieldList)
		})

		replace(&cpy)
	case *ast.FuncType:
		cpy := *n
		if cpy.Params != nil {
			walkToReplace(v, cpy.Params, func(r ast.Node) {
				cpy.Params = r.(*ast.FieldList)
			})
		}
		if cpy.Results != nil {
			walkToReplace(v, cpy.Results, func(r ast.Node) {
				cpy.Results = r.(*ast.FieldList)
			})
		}

		replace(&cpy)
	case *ast.InterfaceType:
		cpy := *n
		walkToReplace(v, cpy.Methods, func(r ast.Node) {
			cpy.Methods = r.(*ast.FieldList)
		})

		replace(&cpy)
	case *ast.MapType:
		cpy := *n
		walkToReplace(v, cpy.Key, func(r ast.Node) {
			cpy.Key = r.(ast.Expr)
		})
		walkToReplace(v, cpy.Value, func(r ast.Node) {
			cpy.Value = r.(ast.Expr)
		})

		replace(&cpy)
	case *ast.ChanType:
		cpy := *n
		walkToReplace(v, cpy.Value, func(r ast.Node) {
			cpy.Value = r.(ast.Expr)
		})

		replace(&cpy)
	case *ast.DeclStmt:
		cpy := *n
		walkToReplace(v, cpy.Decl, func(r ast.Node) {
			cpy.Decl = r.(ast.Decl)
		})

		replace(&cpy)
	case *ast.LabeledStmt:
		cpy := *n
		walkToReplace(v, cpy.Label, func(r ast.Node) {
			cpy.Label = r.(*ast.Ident)
		})
		walkToReplace(v, cpy.Stmt, func(r ast.Node) {
			cpy.Stmt = r.(ast.Stmt)
		})

		replace(&cpy)
	case *ast.ExprStmt:
		cpy := *n
		walkToReplace(v, cpy.X, func(r ast.Node) {
			cpy.X = r.(ast.Expr)
		})

		replace(&cpy)
	case *ast.SendStmt:
		cpy := *n
		walkToReplace(v, cpy.Chan, func(r ast.Node) {
			cpy.Chan = r.(ast.Expr)
		})
		walkToReplace(v, cpy.Value, func(r ast.Node) {
			cpy.Value = r.(ast.Expr)
		})

		replace(&cpy)
	case *ast.IncDecStmt:
		cpy := *n
		walkToReplace(v, cpy.X, func(r ast.Node) {
			cpy.X = r.(ast.Expr)
		})

		replace(&cpy)
	case *ast.AssignStmt:
		cpy := *n
		if n.Lhs != nil {
			cpy.Lhs = make([]ast.Expr, len(n.Lhs))
			copy(cpy.Lhs, n.Lhs)
		}
		if n.Rhs != nil {
			cpy.Rhs = make([]ast.Expr, len(n.Rhs))
			copy(cpy.Rhs, n.Rhs)
		}

		walkExprList(v, cpy.Lhs)
		walkExprList(v, cpy.Rhs)

		replace(&cpy)
	case *ast.GoStmt:
		cpy := *n
		walkToReplace(v, cpy.Call, func(r ast.Node) {
			cpy.Call = r.(*ast.CallExpr)
		})

		replace(&cpy)
	case *ast.DeferStmt:
		cpy := *n
		walkToReplace(v, cpy.Call, func(r ast.Node) {
			cpy.Call = r.(*ast.CallExpr)
		})

		replace(&cpy)
	case *ast.ReturnStmt:
		cpy := *n
		if n.Results != nil {
			cpy.Results = make([]ast.Expr, len(n.Results))
			copy(cpy.Results, n.Results)
		}

		walkExprList(v, cpy.Results)

		replace(&cpy)
	case *ast.BranchStmt:
		cpy := *n
		if cpy.Label != nil {
			walkToReplace(v, cpy.Label, func(r ast.Node) {
				cpy.Label = r.(*ast.Ident)
			})
		}

		replace(&cpy)
	case *ast.BlockStmt:
		cpy := *n
		if n.List != nil {
			cpy.List = make([]ast.Stmt, len(n.List))
			copy(cpy.List, n.List)
		}

		walkStmtList(v, cpy.List)

		replace(&cpy)
	case *ast.IfStmt:
		cpy := *n

		if cpy.Init != nil {
			walkToReplace(v, cpy.Init, func(r ast.Node) {
				cpy.Init = r.(ast.Stmt)
			})
		}
		walkToReplace(v, cpy.Cond, func(r ast.Node) {
			cpy.Cond = r.(ast.Expr)
		})
		walkToReplace(v, cpy.Body, func(r ast.Node) {
			cpy.Body = r.(*ast.BlockStmt)
		})
		if cpy.Else != nil {
			walkToReplace(v, cpy.Else, func(r ast.Node) {
				cpy.Else = r.(ast.Stmt)
			})
		}

		replace(&cpy)
	case *ast.CaseClause:
		cpy := *n
		if n.List != nil {
			cpy.List = make([]ast.Expr, len(n.List))
			copy(cpy.List, n.List)
		}
		if n.Body != nil {
			cpy.Body = make([]ast.Stmt, len(n.Body))
			copy(cpy.Body, n.Body)
		}

		walkExprList(v, cpy.List)
		walkStmtList(v, cpy.Body)

		replace(&cpy)
	case *ast.SwitchStmt:
		cpy := *n
		if cpy.Init != nil {
			walkToReplace(v, cpy.Init, func(r ast.Node) {
				cpy.Init = r.(ast.Stmt)
			})
		}
		if cpy.Tag != nil {
			walkToReplace(v, cpy.Tag, func(r ast.Node) {
				cpy.Tag = r.(ast.Expr)
			})
		}
		walkToReplace(v, cpy.Body, func(r ast.Node) {
			cpy.Body = r.(*ast.BlockStmt)
		})

		replace(&cpy)
	case *ast.TypeSwitchStmt:
		cpy := *n
		if cpy.Init != nil {
			walkToReplace(v, cpy.Init, func(r ast.Node) {
				cpy.Init = r.(ast.Stmt)
			})
		}
		walkToReplace(v, cpy.Assign, func(r ast.Node) {
			cpy.Assign = r.(ast.Stmt)
		})
		walkToReplace(v, cpy.Body, func(r ast.Node) {
			cpy.Body = r.(*ast.BlockStmt)
		})

		replace(&cpy)
	case *ast.CommClause:
		cpy := *n
		if n.Body != nil {
			cpy.Body = make([]ast.Stmt, len(n.Body))
			copy(cpy.Body, n.Body)
		}

		if cpy.Comm != nil {
			walkToReplace(v, cpy.Comm, func(r ast.Node) {
				cpy.Comm = r.(ast.Stmt)
			})
		}
		walkStmtList(v, cpy.Body)

		replace(&cpy)
	case *ast.SelectStmt:
		cpy := *n
		walkToReplace(v, cpy.Body, func(r ast.Node) {
			cpy.Body = r.(*ast.BlockStmt)
		})

		replace(&cpy)
	case *ast.ForStmt:
		cpy := *n
		if cpy.Init != nil {
			walkToReplace(v, cpy.Init, func(r ast.Node) {
				cpy.Init = r.(ast.Stmt)
			})
		}
		if cpy.Cond != nil {
			walkToReplace(v, cpy.Cond, func(r ast.Node) {
				cpy.Cond = r.(ast.Expr)
			})
		}
		if cpy.Post != nil {
			walkToReplace(v, cpy.Post, func(r ast.Node) {
				cpy.Post = r.(ast.Stmt)
			})
		}
		walkToReplace(v, cpy.Body, func(r ast.Node) {
			cpy.Body = r.(*ast.BlockStmt)
		})

		replace(&cpy)
	case *ast.RangeStmt:
		cpy := *n
		if cpy.Key != nil {
			walkToReplace(v, cpy.Key, func(r ast.Node) {
				cpy.Key = r.(ast.Expr)
			})
		}
		if cpy.Value != nil {
			walkToReplace(v, cpy.Value, func(r ast.Node) {
				cpy.Value = r.(ast.Expr)
			})
		}
		walkToReplace(v, cpy.X, func(r ast.Node) {
			cpy.X = r.(ast.Expr)
		})
		walkToReplace(v, cpy.Body, func(r ast.Node) {
			cpy.Body = r.(*ast.BlockStmt)
		})

		// Declarations
		replace(&cpy)
	case *ast.ImportSpec:
		cpy := *n
		if cpy.Doc != nil {
			walkToReplace(v, cpy.Doc, func(r ast.Node) {
				cpy.Doc = r.(*ast.CommentGroup)
			})
		}
		if cpy.Name != nil {
			walkToReplace(v, cpy.Name, func(r ast.Node) {
				cpy.Name = r.(*ast.Ident)
			})
		}
		walkToReplace(v, cpy.Path, func(r ast.Node) {
			cpy.Path = r.(*ast.BasicLit)
		})
		if cpy.Comment != nil {
			walkToReplace(v, cpy.Comment, func(r ast.Node) {
				cpy.Comment = r.(*ast.CommentGroup)
			})
		}

		replace(&cpy)
	case *ast.ValueSpec:
		cpy := *n
		if n.Names != nil {
			cpy.Names = make([]*ast.Ident, len(n.Names))
			copy(cpy.Names, n.Names)
		}
		if n.Values != nil {
			cpy.Values = make([]ast.Expr, len(n.Values))
			copy(cpy.Values, n.Values)
		}

		if cpy.Doc != nil {
			walkToReplace(v, cpy.Doc, func(r ast.Node) {
				cpy.Doc = r.(*ast.CommentGroup)
			})
		}

		walkIdentList(v, cpy.Names)

		if cpy.Type != nil {
			walkToReplace(v, cpy.Type, func(r ast.Node) {
				cpy.Type = r.(ast.Expr)
			})
		}

		walkExprList(v, cpy.Values)

		if cpy.Comment != nil {
			walkToReplace(v, cpy.Comment, func(r ast.Node) {
				cpy.Comment = r.(*ast.CommentGroup)
			})
		}

		replace(&cpy)

	case *ast.TypeSpec:
		cpy := *n

		if cpy.Doc != nil {
			walkToReplace(v, cpy.Doc, func(r ast.Node) {
				cpy.Doc = r.(*ast.CommentGroup)
			})
		}
		walkToReplace(v, cpy.Name, func(r ast.Node) {
			cpy.Name = r.(*ast.Ident)
		})
		walkToReplace(v, cpy.Type, func(r ast.Node) {
			cpy.Type = r.(ast.Expr)
		})
		if cpy.Comment != nil {
			walkToReplace(v, cpy.Comment, func(r ast.Node) {
				cpy.Comment = r.(*ast.CommentGroup)
			})
		}

		replace(&cpy)
	case *ast.GenDecl:
		cpy := *n
		if n.Specs != nil {
			cpy.Specs = make([]ast.Spec, len(n.Specs))
			copy(cpy.Specs, n.Specs)
		}

		if cpy.Doc != nil {
			walkToReplace(v, cpy.Doc, func(r ast.Node) {
				cpy.Doc = r.(*ast.CommentGroup)
			})
		}
		for i, s := range cpy.Specs {
			walkToReplace(v, s, func(r ast.Node) {
				cpy.Specs[i] = r.(ast.Spec)
			})
		}

		replace(&cpy)
	case *ast.FuncDecl:
		cpy := *n

		if cpy.Doc != nil {
			walkToReplace(v, cpy.Doc, func(r ast.Node) {
				cpy.Doc = r.(*ast.CommentGroup)
			})
		}
		if cpy.Recv != nil {
			walkToReplace(v, cpy.Recv, func(r ast.Node) {
				cpy.Recv = r.(*ast.FieldList)
			})
		}
		walkToReplace(v, cpy.Name, func(r ast.Node) {
			cpy.Name = r.(*ast.Ident)
		})
		walkToReplace(v, cpy.Type, func(r ast.Node) {
			cpy.Type = r.(*ast.FuncType)
		})
		if cpy.Body != nil {
			walkToReplace(v, cpy.Body, func(r ast.Node) {
				cpy.Body = r.(*ast.BlockStmt)
			})
		}

		// Files and packages
		replace(&cpy)
	case *ast.File:
		cpy := *n

		if cpy.Doc != nil {
			walkToReplace(v, cpy.Doc, func(r ast.Node) {
				cpy.Doc = r.(*ast.CommentGroup)
			})
		}
		walkToReplace(v, cpy.Name, func(r ast.Node) {
			cpy.Name = r.(*ast.Ident)
		})
		walkDeclList(v, cpy.Decls)
		// don't walk cpy.Comments - they have been
		// visited already through the individual
		// nodes

		replace(&cpy)
	case *ast.Package:
		cpy := *n
		cpy.Files = map[string]*ast.File{}

		for i, f := range n.Files {
			cpy.Files[i] = f
			walkToReplace(v, f, func(r ast.Node) {
				cpy.Files[i] = r.(*ast.File)
			})
		}
		replace(&cpy)

	default:
		panic(fmt.Sprintf("walkToReplace: unexpected node type %T", n))
	}

	if v != nil {
		v.Visit(nil, func(ast.Node) { panic("can't replace the go-up nil") })
	}
}
