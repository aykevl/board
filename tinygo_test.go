package board_test

import (
	"bytes"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"testing"
)

var boards = []string{
	// Please keep this list sorted!
	"gameboy-advance",
	"gopher-badge",
	"mch2022",
	"pinetime",
	"pybadge",
	"pyportal",
	"simulator",
	"thumby",
}

func isXtensa(board string) bool {
	return board == "mch2022"
}

var flagXtensa = flag.Bool("xtensa", false, "test Xtensa based boards")

// These method names should match the ones in testdata/smoketest.go, so that no
// method goes unchecked!
var definedGlobals = map[string][]string{
	"Power": []string{
		"Configure",
		"Status",
	},
	"Sensors": []string{
		"Configure",
		"Update",
		"Acceleration",
		"Steps",
		"Temperature",
	},
	"Display": []string{
		"Configure",
		"PPI",
		"ConfigureTouch",
		"MaxBrightness",
		"SetBrightness",
		"WaitForVBlank",
	},
	"Buttons": []string{
		"Configure",
		"ReadInput",
		"NextEvent",
	},
}

func TestBoards(t *testing.T) {
	for _, board := range boards {
		board := board
		t.Run(board, func(t *testing.T) {
			if isXtensa(board) && !*flagXtensa {
				t.Skip("skipping Xtensa board:", board)
			}
			t.Parallel()
			outbuf := &bytes.Buffer{}
			var cmd *exec.Cmd
			if board == "simulator" {
				cmd = exec.Command("go", "build", "-o="+t.TempDir()+"/output", "./testdata/smoketest.go")
			} else {
				cmd = exec.Command("tinygo", "build", "-o="+t.TempDir()+"/output", "-target="+board, "./testdata/smoketest.go")
			}
			cmd.Stderr = outbuf
			cmd.Stdout = outbuf
			err := cmd.Run()
			if err != nil {
				t.Errorf("failed to compile smoke test: %s\n%s", err, outbuf.String())
			}
		})
	}
}

// Test for exported names: all of them have to adhere to a strict API so that
// the API for all boards is the same.
func TestExported(t *testing.T) {
	for _, board := range boards {
		board := board
		t.Run(board, func(t *testing.T) {
			// Parse the Go file into an AST.
			filename := "board-" + board + ".go"
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, filename, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Errorf("could not open/parse %s: %v", filename, err)
			}

			// Collect method names for (typically unexported) named types.
			// Also set some defaults that aren't defined in board files (but in
			// common.go, probably).
			methodNames := map[string][]string{
				"baseSensors":  definedGlobals["Sensors"],
				"dummyBattery": definedGlobals["Power"],
				"noButtons":    definedGlobals["Buttons"],
			}
			for _, decl := range f.Decls {
				if decl, ok := decl.(*ast.FuncDecl); ok {
					if decl.Name.IsExported() && decl.Recv != nil && len(decl.Recv.List) > 0 {
						recvType := decl.Recv.List[0].Type
						name := extractTypeName(recvType)
						methodNames[name] = append(methodNames[name], decl.Name.Name)
					}
				}
			}

			// Check all exported types, variables, etc.
			for _, decl := range f.Decls {
				pos := fset.Position(decl.Pos())
				switch decl := decl.(type) {
				case *ast.FuncDecl:
					if !decl.Name.IsExported() {
						continue
					}
					if decl.Recv != nil && len(decl.Recv.List) > 0 {
						// Method name, this is checked when checking named
						// types.
						continue
					}
					t.Errorf("%s: unexpected exported function %s", pos, decl.Name.Name)
				case *ast.GenDecl:
					switch decl.Tok {
					case token.IMPORT:
						// imports don't export anything
					case token.CONST:
						for _, spec := range decl.Specs {
							pos := fset.Position(spec.Pos())
							switch spec := spec.(type) {
							case *ast.ValueSpec:
								for _, name := range spec.Names {
									if !name.IsExported() {
										continue
									}
									if name.Name != "Name" {
										// "Name" is the only allowed constant.
										t.Errorf("%s: unexpected constant: %s", pos, name.Name)
									}
								}
							default:
								t.Errorf("%s: unexpected spec: %#v", pos, spec)
							}
						}
					case token.VAR:
						// Variables are things like Power, Display, etc that
						// are typically defined using unexported named types.
						// We need to check that they don't define any unexpected methods.
						for _, spec := range decl.Specs {
							pos := fset.Position(spec.Pos())
							switch spec := spec.(type) {
							case *ast.ValueSpec:
								for _, name := range spec.Names {
									if !name.IsExported() {
										continue
									}
									if _, ok := definedGlobals[name.Name]; !ok {
										t.Errorf("%s: unexpected variable: %s", pos, name.Name)
										continue
									}
									if len(spec.Values) != 1 {
										t.Errorf("%s: expected a single value for board.%s", pos, name.Name)
										continue
									}
									typeName := extractTypeName(spec.Values[0])
									if _, ok := methodNames[typeName]; !ok {
										t.Errorf("%s: could not find methods for type %#v", pos, typeName)
										continue
									}
									for _, typeMethod := range methodNames[typeName] {
										found := false
										for _, expectedMethod := range definedGlobals[name.Name] {
											if typeMethod == expectedMethod {
												found = true
											}
										}
										if !found {
											t.Errorf("%s: unexpected method %s on board.%s", pos, typeMethod, name)
										}
									}
								}
							default:
								t.Errorf("%s: unexpected spec: %#v", pos, spec)
							}
						}
					case token.TYPE:
						// Boards shouldn't define any new types.
						for _, spec := range decl.Specs {
							pos := fset.Position(spec.Pos())
							switch spec := spec.(type) {
							case *ast.TypeSpec:
								if !spec.Name.IsExported() {
									continue
								}
								t.Errorf("%s: unexpected type: %s", pos, spec.Name)
							default:
								t.Errorf("%s: unexpected spec: %#v", pos, spec)
							}
						}
					default:
						t.Errorf("%s: unexpected declaration: %#v", pos, decl)
					}
				default:
					t.Logf("%s: unexpected declaration: %#v", pos, decl)
				}
			}
		})
	}
}

// Extract the named type from the given AST expression (resolving things like
// *ast.StarExpr).
func extractTypeName(x ast.Expr) string {
	switch value := x.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.CompositeLit:
		return extractTypeName(value.Type)
	case *ast.StarExpr:
		return extractTypeName(value.X)
	case *ast.UnaryExpr:
		return extractTypeName(value.X)
	default:
		return "<unknown>"
	}
}
