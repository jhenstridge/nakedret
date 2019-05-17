package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

const (
	pwd = "./"
)

func init() {
	//TODO allow build tags
	build.Default.UseAllFiles = true
}

func usage() {
	log.Printf("Usage of %s:\n", os.Args[0])
	log.Printf("\nnakedret [flags] # runs on package in current directory\n")
	log.Printf("\nnakedret [flags] [packages]\n")
	log.Printf("Flags:\n")
	flag.PrintDefaults()
}

type returnsVisitor struct {
	f         *token.FileSet
	maxLength uint
}

func main() {

	// Remove log timestamp
	log.SetFlags(0)

	maxLength := flag.Uint("l", 5, "maximum number of lines for a naked return function")
	flag.Usage = usage
	flag.Parse()
	i := rand.Int()

	if err := checkNakedReturns(flag.Args(), maxLength, i); err != nil {
		log.Println(err)
	}
}

type f struct{}

func (f) Lol() {}

func checkNakedReturns(args []string, maxLength *uint, unused int) error {

	myF := f{}
	myF.Lol()

	fset := token.NewFileSet()

	files, err := parseInput(args, fset)
	if err != nil {
		return fmt.Errorf("could not parse input %v", err)
	}

	if maxLength == nil {
		return errors.New("max length nil")
	}

	retVis := &returnsVisitor{
		f:         fset,
		maxLength: *maxLength,
	}

	for _, f := range files {
		ast.Walk(retVis, f)
	}

	return nil
}

func parseInput(args []string, fset *token.FileSet) ([]*ast.File, error) {
	var directoryList []string
	var fileMode bool
	files := make([]*ast.File, 0)

	if len(args) == 0 {
		directoryList = append(directoryList, pwd)
	} else {
		for _, arg := range args {
			if strings.HasSuffix(arg, "/...") && isDir(arg[:len(arg)-len("/...")]) {

				for _, dirname := range allPackagesInFS(arg) {
					directoryList = append(directoryList, dirname)
				}

			} else if isDir(arg) {
				directoryList = append(directoryList, arg)

			} else if exists(arg) {
				if strings.HasSuffix(arg, ".go") {
					fileMode = true
					f, err := parser.ParseFile(fset, arg, nil, 0)
					if err != nil {
						return nil, err
					}
					files = append(files, f)
				} else {
					return nil, fmt.Errorf("invalid file %v specified", arg)
				}
			} else {

				//TODO clean this up a bit
				imPaths := importPaths([]string{arg})
				for _, importPath := range imPaths {
					pkg, err := build.Import(importPath, ".", 0)
					if err != nil {
						return nil, err
					}
					var stringFiles []string
					stringFiles = append(stringFiles, pkg.GoFiles...)
					// files = append(files, pkg.CgoFiles...)
					stringFiles = append(stringFiles, pkg.TestGoFiles...)
					if pkg.Dir != "." {
						for i, f := range stringFiles {
							stringFiles[i] = filepath.Join(pkg.Dir, f)
						}
					}

					fileMode = true
					for _, stringFile := range stringFiles {
						f, err := parser.ParseFile(fset, stringFile, nil, 0)
						if err != nil {
							return nil, err
						}
						files = append(files, f)
					}

				}
			}
		}
	}

	// if we're not in file mode, then we need to grab each and every package in each directory
	// we can to grab all the files
	if !fileMode {
		for _, fpath := range directoryList {
			pkgs, err := parser.ParseDir(fset, fpath, nil, 0)
			if err != nil {
				return nil, err
			}

			for _, pkg := range pkgs {
				for _, f := range pkg.Files {
					files = append(files, f)
				}
			}
		}
	}

	return files, nil
}

func isDir(filename string) bool {
	fi, err := os.Stat(filename)
	return err == nil && fi.IsDir()
}

func exists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// Other ideas - see if you can see where returns are being ignored i.e. client.Close() should be _ = client.Close()
// AssignStmt - LHS is empty?

//TODO - could also look for methods with receivers that don't actually use the receiver? eh

func (v *returnsVisitor) Visit(node ast.Node) ast.Visitor {

	// os.Stat("hihiihihihi")

	// // search for call expressions
	// assignStmt, ok := node.(*ast.AssignStmt)
	// if !ok {
	// 	return v
	// }

	// file := v.f.File(assignStmt.Pos())
	// // fmt.Printf("%v:%v got one %T\n", file.Name(), file.Position(assignStmt.Pos()).Line, assignStmt.Fun)

	// fmt.Printf("%v:%v got one\n", file.Name(), file.Position(assignStmt.Pos()).Line)
	// fmt.Printf("I have a LHS assignment with size %+v\n", len(assignStmt.Lhs))

	// Next up is to check if there are any receivers

	// search for call expressions
	funcDecl, ok := node.(*ast.FuncDecl)
	if !ok {
		return v
	}

	paramMap := make(map[string]bool)

	if funcDecl.Type != nil && funcDecl.Type.Params != nil {
		for _, paramList := range funcDecl.Type.Params.List {
			for _, name := range paramList.Names {
				if name.Name == "_" {
					continue
				}
				paramMap[name.Name] = false
			}
		}
	}

	// fmt.Printf("%v::: %v\n", funcDecl.Name.Name, paramMap)
	if len(paramMap) == 0 {
		return v
	}

	file := v.f.File(funcDecl.Pos())

	// Analyze body of function
	for len(funcDecl.Body.List) != 0 {
		// log.Printf("--------------%v %T\n", stmt.
		stmt := funcDecl.Body.List[0]

		switch s := stmt.(type) {
		case *ast.IfStmt:
			// Either variable is in condition or body
			funcDecl.Body.List = append(funcDecl.Body.List, s.Body, s.Init, s.Else)
			funcDecl.Body.List = processExpr(paramMap, []ast.Expr{s.Cond}, funcDecl.Body.List)

		case *ast.AssignStmt:
			//TODO left and right sides?
			funcDecl.Body.List = processExpr(paramMap, s.Lhs, funcDecl.Body.List)
			funcDecl.Body.List = processExpr(paramMap, s.Rhs, funcDecl.Body.List)

		case *ast.BlockStmt:
			funcDecl.Body.List = append(funcDecl.Body.List, s.List...)

		case *ast.ReturnStmt:
			funcDecl.Body.List = processExpr(paramMap, s.Results, funcDecl.Body.List)

		case *ast.DeclStmt:
			switch d := s.Decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					//TODO - i think we only care about valuespec here

					switch specType := spec.(type) {
					case *ast.ValueSpec:
						handleIdents(paramMap, specType.Names)
						funcDecl.Body.List = processExpr(paramMap, []ast.Expr{specType.Type}, funcDecl.Body.List)
						funcDecl.Body.List = processExpr(paramMap, specType.Values, funcDecl.Body.List)

					}

				}

			default:
				fmt.Printf("## decl type not handled %T\n", d)
			}

		case *ast.ExprStmt:
			funcDecl.Body.List = processExpr(paramMap, []ast.Expr{s.X}, funcDecl.Body.List)

		case *ast.RangeStmt:
			funcDecl.Body.List = append(funcDecl.Body.List, s.Body)
			funcDecl.Body.List = processExpr(paramMap, []ast.Expr{s.X}, funcDecl.Body.List)

		case *ast.ForStmt:
			funcDecl.Body.List = append(funcDecl.Body.List, s.Body)
			funcDecl.Body.List = processExpr(paramMap, []ast.Expr{s.Cond}, funcDecl.Body.List)

			funcDecl.Body.List = append(funcDecl.Body.List, s.Post)

		case *ast.TypeSwitchStmt:
			funcDecl.Body.List = append(funcDecl.Body.List, s.Body, s.Assign, s.Init)

		case *ast.CaseClause:
			funcDecl.Body.List = processExpr(paramMap, s.List, funcDecl.Body.List)

			funcDecl.Body.List = append(funcDecl.Body.List, s.Body...)

		case *ast.SendStmt:
			funcDecl.Body.List = processExpr(paramMap, []ast.Expr{s.Chan, s.Value}, funcDecl.Body.List)

		case *ast.GoStmt:
			funcDecl.Body.List = processExpr(paramMap, []ast.Expr{s.Call}, funcDecl.Body.List)

		case *ast.DeferStmt:
			funcDecl.Body.List = processExpr(paramMap, []ast.Expr{s.Call}, funcDecl.Body.List)

		case *ast.SelectStmt:
			funcDecl.Body.List = append(funcDecl.Body.List, s.Body)

		case *ast.CommClause:
			funcDecl.Body.List = append(funcDecl.Body.List, s.Body...)
			funcDecl.Body.List = append(funcDecl.Body.List, s.Comm)

		case *ast.BranchStmt:
			handleIdent(paramMap, s.Label)

		case *ast.SwitchStmt:
			funcDecl.Body.List = append(funcDecl.Body.List, s.Body, s.Init)
			funcDecl.Body.List = processExpr(paramMap, []ast.Expr{s.Tag}, funcDecl.Body.List)

		case *ast.LabeledStmt:
			// this one is kinda weird
			handleIdent(paramMap, s.Label)
			funcDecl.Body.List = append(funcDecl.Body.List, s.Stmt)

		case nil, *ast.IncDecStmt:
			//no-op

		default:
			// nils will happen here without nil checks on my appends, meh
			fmt.Printf("~~~~ missing type %T\n", s)

		}

		funcDecl.Body.List = funcDecl.Body.List[1:]
	}

	for key, val := range paramMap {
		if !val {
			if file != nil {
				if funcDecl.Name != nil {
					log.Printf("--------------%v:%v %v found unnn  ------------\n", file.Name(), file.Position(funcDecl.Pos()).Line, funcDecl.Name.Name)
				}
			}
			fmt.Printf("noooooooooooooooooooo %v\n", key)
		} else {
			// fmt.Printf("yesss %v\n", key)
		}
	}

	return v
}

func handleIdents(paramMap map[string]bool, identList []*ast.Ident) {
	for _, ident := range identList {
		handleIdent(paramMap, ident)
	}
}

func handleIdent(paramMap map[string]bool, ident *ast.Ident) {
	if ident == nil {
		return
	}
	if _, ok := paramMap[ident.Name]; ok {
		paramMap[ident.Name] = true
	}
}

func processExpr(paramMap map[string]bool, exprList []ast.Expr, stmtList []ast.Stmt) []ast.Stmt {
	for len(exprList) != 0 {
		expr := exprList[0]
		switch e := expr.(type) {
		case *ast.Ident:
			handleIdent(paramMap, e)
		case *ast.BinaryExpr:
			exprList = append(exprList, e.X) //TODO, do we need to then worry about x.left being used?
			exprList = append(exprList, e.Y) //TODO, do we need to then worry about x.left being used?
		case *ast.FuncLit:
			stmtList = append(stmtList, e.Body)
		case *ast.BasicLit:
			// nothing to do here, no variable name
		case *ast.SelectorExpr:
			exprList = append(exprList, e.X)
			handleIdent(paramMap, e.Sel)
		case *ast.CompositeLit:
			exprList = append(exprList, e.Elts...)

		case *ast.CallExpr:
			exprList = append(exprList, e.Args...)
			exprList = append(exprList, e.Fun)

		case *ast.IndexExpr:
			exprList = append(exprList, e.X)
			exprList = append(exprList, e.Index)

		case *ast.SliceExpr:
			exprList = append(exprList, e.Low, e.High, e.Max, e.X)

		case *ast.TypeAssertExpr:
			exprList = append(exprList, e.X, e.Type)

		case *ast.StarExpr:
			exprList = append(exprList, e.X)

		case *ast.UnaryExpr:
			exprList = append(exprList, e.X)

		case *ast.MapType:
			exprList = append(exprList, e.Key, e.Value)

		case *ast.KeyValueExpr:
			exprList = append(exprList, e.Key, e.Value)

		case *ast.ArrayType:
			//TODO - is len needed here?
			exprList = append(exprList, e.Elt, e.Len)

			//TODO - struct type and interface type are trouble with fieldList

		case *ast.ChanType:
			exprList = append(exprList, e.Value)

		case *ast.ParenExpr:
			exprList = append(exprList, e.X)

		case *ast.StructType:
			exprList, stmtList = processFieldList(paramMap, e.Fields, exprList, stmtList)

		case *ast.InterfaceType:
			exprList, stmtList = processFieldList(paramMap, e.Methods, exprList, stmtList)

		case nil:
			// no op

		default:
			fmt.Printf("@@@@@@@@@@ missing type %T\n", e)
		}
		exprList = exprList[1:]
	}

	return stmtList
}

func processFieldList(paramMap map[string]bool, fieldList *ast.FieldList, exprList []ast.Expr, stmtList []ast.Stmt) ([]ast.Expr, []ast.Stmt) {

	for _, field := range fieldList.List {
		exprList = append(exprList, field.Type)
		handleIdents(paramMap, field.Names)

		// don't care about Tag, need to handle ident and expr
	}

	return exprList, stmtList
}
