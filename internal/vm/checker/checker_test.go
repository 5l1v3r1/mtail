// Copyright 2016 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package checker_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/mtail/internal/testutil"
	"github.com/google/mtail/internal/vm/ast"
	"github.com/google/mtail/internal/vm/checker"
	"github.com/google/mtail/internal/vm/parser"
	"github.com/google/mtail/internal/vm/symbol"
	"github.com/google/mtail/internal/vm/types"
)

var checkerInvalidPrograms = []struct {
	name    string
	program string
	errors  []string
}{
	{"undefined named capture group",
		"/blurgh/ { $undef++\n }\n",
		[]string{"undefined named capture group:1:12-17: Capture group `$undef' was not defined by a regular expression visible to this scope.", "\tTry using `(?P<undef>...)' to name the capture group."}},

	{"out of bounds capref",
		"/(blyurg)/ { $2++ \n}\n",
		[]string{"out of bounds capref:1:14-15: Capture group `$2' was not defined by a regular expression " +
			"visible to this scope.", "\tCheck that there are at least 2 pairs of parentheses."},
	},

	{"undefined decorator",
		"@foo {}\n",
		[]string{"undefined decorator:1:1-4: Decorator `foo' not defined.", "\tTry adding a definition `def foo {}' earlier in the program."}},

	{"undefined identifier",
		"// { x++ \n}\n",
		[]string{"undefined identifier:1:6: Identifier `x' not declared.", "\tTry adding `counter x' to the top of the program."},
	},

	{"invalid regex 1",
		"/foo(/ {}\n",
		[]string{"invalid regex 1:1:1-6: error parsing regexp: missing closing ): `foo(`"}},

	{"invalid regex 2",
		"/blurg(?P<x.)/ {}\n",
		[]string{"invalid regex 2:1:1-14: error parsing regexp: invalid named capture: `(?P<x.)`"}},

	{"invalid regex 3",
		"/blurg(?P<x>[[:alph:]])/ {}\n",
		[]string{"invalid regex 3:1:1-24: error parsing regexp: invalid character class range: `[:alph:]`"}},

	{"duplicate declaration",
		"counter foo\ncounter foo\n",
		[]string{"duplicate declaration:2:9-11: Redeclaration of metric `foo' previously declared at duplicate declaration:1:9-11",
			"duplicate declaration:1:9-11: Declaration of variable `foo' is never used"}},

	{"indexedExpr parameter count",
		`counter n
    counter foo by a, b
	counter bar by a, b
	counter quux by a
	/(\d+)/ {
      n[$1]++
      foo[$1]++
      bar[$1][0]++
      quux[$1][0]++
	}
		`,
		[]string{
			// n[$1] is syntactically valid, but n is not indexable
			"indexedExpr parameter count:6:7-10: Index taken on unindexable expression",
			// foo[$1] is short one key
			"indexedExpr parameter count:7:7-12: Not enough keys for indexed expression: expecting 2, received 1",
			// bar[$1][0] is ok
			// quux[$1][0] has too many keys
			"indexedExpr parameter count:9:7-16: Too many keys for indexed expression: expecting 1, received 2.",
		}},

	{"indexedExpr binary expression",
		`counter foo by a, b
counter bar by a, b
/(\d+)/ {
  foo[$1]+=$1
}
/(.*)/ {
  foo = bar[$1] + 1
}
`,
		[]string{
			"indexedExpr binary expression:4:3-8: Not enough keys for indexed expression: expecting 2, received 1",
			"indexedExpr binary expression:7:3-5: Not enough keys for indexed expression: expecting 2, received 0",
			"indexedExpr binary expression:7:9-14: Not enough keys for indexed expression: expecting 2, received 1",
		}},

	{"builtin parameter mismatch",
		`/\d+/ {
	  strptime()
	}
    /\d+/ {
	  timestamp()
	}
	`,
		[]string{"builtin parameter mismatch:2:13: call to `strptime': type mismatch; expected String→String→None received incomplete type"}},

	{"bad strptime format",
		`strptime("2017-10-16 06:50:25", "2017-10-16 06:50:25")
`,
		[]string{
			"bad strptime format:1:33-53: invalid time format string \"2017-10-16 06:50:25\"", "\tRefer to the documentation at https://golang.org/pkg/time/#pkg-constants for advice."}},

	{"undefined const regex",
		"/foo / + X + / bar/ {}\n",
		[]string{"undefined const regex:1:10: Identifier `X' not declared.", "\tTry adding `const X /.../' earlier in the program."}},

	{"unused symbols",
		`counter foo
const ID /bar/
/asdf/ {
}
`,
		[]string{"unused symbols:1:9-11: Declaration of variable `foo' is never used",
			"unused symbols:2:7-8: Declaration of named pattern constant `ID' is never used"}},
	{"invalid del index count",
		`gauge t by x, y
/.*/ {
  del t["x"]
  t["x"]["y"]
}
`,
		[]string{"invalid del index count:3:7-11: Not enough keys for indexed expression: expecting 2, received 1"}},
	// TODO(jaq): is it an error to make a counter of type string?
	// 	{"counter as string",
	// 		`counter foo

	// /(?P<v>.*)/ {
	//   foo = $v
	// }
	// `,
	// 	[]string{"counter as string:4:4-11: Can't assign rhs of type String to lhs of type Int"}},
	{"def without usage",
		`def x{next}`,
		[]string{"def without usage:1:1-10: Declaration of decorator `x' is never used"}},
	{"def without next",
		`def x{}
@x {
}`,
		[]string{"def without next:1:1-3: No symbols found in decorator `@x', try adding a `next' statement."}},
	{"def with two nexts",
		`def x{
  /a/ {
    next
  }
  /b/ {
    next
  }
}
@x {
}`,
		[]string{"def with two nexts:6:5-8: Can't use `next' statement twice in a decorator."}},
}

func TestCheckInvalidPrograms(t *testing.T) {
	for _, tc := range checkerInvalidPrograms {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ast, err := parser.Parse(tc.name, strings.NewReader(tc.program))
			if err != nil {
				t.Fatal(err)
			}
			ast, err = checker.Check(ast)
			if err == nil {
				s := parser.Sexp{}
				s.EmitTypes = true
				t.Log(s.Dump(ast))
				t.Fatal("check didn't fail")
			}

			diff := testutil.Diff(
				tc.errors,                        // want
				strings.Split(err.Error(), "\n"), // got
				cmpopts.SortSlices(func(x, y string) bool { return x < y }))
			if diff != "" {
				t.Errorf("Diff %s", diff)
				t.Logf("Got: %s", err.Error())
				s := parser.Sexp{}
				s.EmitTypes = true
				t.Log(s.Dump(ast))
			}
		})
	}
}

var checkerValidPrograms = []struct {
	name    string
	program string
}{
	{"capture group",
		`counter foo
/(.*)/ {
  foo += $1
}
`,
	},
	{"shadowed positionals",
		`counter foo
/(.*)/ {
  foo += $1
  /bar(\d+)/ {
   foo += $1
  }
}
`},
	{"sibling positionals",
		`counter foo
/(.*)/ {
  foo += $1
}
/bar(\d+)/ {
   foo += $1
}
`},

	{"index expression",
		`counter foo by a, b
/(\d)/ {
  foo[1,$1] = 3
}`},
	{"odd indexes",
		`counter foo by a,b,c
	/(\d) (\d)/ {
	  foo[$1,$2][0]++
	}
	`},
	{"implicit int",
		`counter foo
/$/ {
  foo++
}`},
	{"function return value",
		`len("foo") > 0 {}`},
	{"conversions",
		`counter i
	counter f
	/(.*)/ {
	  i = int($1)
	  f = float($1)
	}
	`},

	{"logical operators",
		`0 || 1 {
}
1 && 0 {
}
`},
	{"nested binary conditional",
		`1 != 0 && 0 == 1 {
}
`},
	{"paren expr", `
(0) || (1 && 3) {
}`},

	{"strptime format", `
strptime("2006-01-02 15:04:05", "2006-01-02 15:04:05")
`},

	{"string concat", `
counter f by s
/(.*), (.*)/ {
  f[$1 + $2]++
}
`},
	{"namespace", `
counter test

/(?P<test>.*)/ {
    test++
}
`},
	{"match expr 1", `
/(?P<foo>.*)/ {
  $foo =~ /bar/ {
  }
}`},

	{"capref used in def", `
/(?P<x>\d+)/ && $x > 0 {
}`},
	{"binop compare type conversion", `
gauge var
/(?P<x>\d+) (\d+\.\d+)/ {
  var = $x + $2
}`},
	{"binop arith type conversion", `
gauge var
/(?P<x>\d+) (\d+\.\d+)/ {
  var = $x + $2
}`},

	{"concat expr 1", `
const X /foo/
/bar/ + X {
}`},
	{"concat expr 2", `
const X /foo/
X {
}`},
	{"match expression 3", `
const X /foo/
"a" =~ X {
}
`},
	{"match expr 4", `
/(?P<foo>.{6}) (?P<bar>.*)/ {
  $foo =~ $bar {
  }
}`},
	{"decorator scopes", `
counter a
def decorator {
  /(.).*/ {
    next
  }
}
@decorator {
  $1 == "A" {
    a++
  }
}
`},
	{"concat with add_assign", `
text foo
/(?P<v>.*)/ {
		foo += $v
}
`},

	{"decrement", `
counter i
/.*/ {
  i--
}`},
	{"stop", `
stop
// {
stop
}`},
}

func TestCheckValidPrograms(t *testing.T) {
	for _, tc := range checkerValidPrograms {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ast, err := parser.Parse(tc.name, strings.NewReader(tc.program))
			if err != nil {
				t.Fatal(err)
			}
			ast, err = checker.Check(ast)
			s := parser.Sexp{}
			s.EmitTypes = true
			t.Log("Typed AST:\n" + s.Dump(ast))
			if err != nil {
				t.Errorf("check failed: %s", err)
			}
		})
	}
}

var checkerTypeExpressionTests = []struct {
	name     string
	expr     ast.Node
	expected types.Type
}{
	{"Int + Int -> Int",
		&ast.BinaryExpr{
			Lhs: &ast.IntLit{I: 1},
			Rhs: &ast.IntLit{I: 1},
			Op:  parser.PLUS},
		types.Int,
	},
	{"Int + Float -> Float",
		&ast.BinaryExpr{
			Lhs: &ast.IntLit{I: 1},
			Rhs: &ast.FloatLit{F: 1.0},
			Op:  parser.PLUS},
		types.Float,
	},
	{"⍺ + Float -> Float",
		&ast.BinaryExpr{
			Lhs: &ast.IdTerm{Symbol: &symbol.Symbol{Name: "i", Kind: symbol.VarSymbol, Type: types.NewVariable()}},
			Rhs: &ast.CaprefTerm{Symbol: &symbol.Symbol{Kind: symbol.CaprefSymbol, Type: types.Float}},
			Op:  parser.PLUS},
		types.Float,
	},
}

func TestCheckTypeExpressions(t *testing.T) {
	for _, tc := range checkerTypeExpressionTests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ast, err := checker.Check(tc.expr)
			if err != nil {
				t.Fatalf("check error: %s", err)
			}

			diff := testutil.Diff(tc.expected, ast.Type().Root())
			if diff != "" {
				t.Error(diff)
				s := parser.Sexp{}
				s.EmitTypes = true
				t.Log("Typed AST:\n" + s.Dump(ast))
			}
		})
	}
}
