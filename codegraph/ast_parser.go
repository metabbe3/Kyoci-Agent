package codegraph

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"sync"
)

// FuncInfo represents information about a function
type FuncInfo struct {
	Name     string
	Receiver string
	Params   []string
	Returns  []string
	File     string
	Line     int
	Doc      string
}

// StructInfo represents information about a struct
type StructInfo struct {
	Name    string
	Fields  []FieldInfo
	Methods []string
	File    string
	Line    int
}

// FieldInfo represents information about a struct field
type FieldInfo struct {
	Name string
	Type string
	Tag  string
}

// InterfaceInfo represents information about an interface
type InterfaceInfo struct {
	Name    string
	Methods []string
	File    string
	Line    int
}

// PackageInfo represents information about a package
type PackageInfo struct {
	Name        string
	Path        string
	Imports     []string
	Functions   []FuncInfo
	Structs     []StructInfo
	Interfaces  []InterfaceInfo
}

// CallEdge represents a function call relationship
type CallEdge struct {
	Caller string
	Callee string
	File   string
}

// CodeGraph represents the code graph structure
type CodeGraph struct {
	Packages         map[string]*PackageInfo
	CallGraph        []CallEdge
	Implementations  map[string][]string // interface -> struct names
	mu               sync.RWMutex
	fset             *token.FileSet
}

// GraphStats provides statistics about the code graph
type GraphStats struct {
	Packages   int
	Functions  int
	Structs    int
	Interfaces int
	CallEdges  int
}

// NewCodeGraph creates a new CodeGraph instance
func NewCodeGraph() *CodeGraph {
	return &CodeGraph{
		Packages:        make(map[string]*PackageInfo),
		CallGraph:       make([]CallEdge, 0),
		Implementations: make(map[string][]string),
		fset:            token.NewFileSet(),
	}
}

// ParseDir walks a directory and parses all .go files
func (g *CodeGraph) ParseDir(dir string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Find all .go files in the directory
	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return fmt.Errorf("failed to find .go files: %w", err)
	}

	// Parse each file
	for _, file := range goFiles {
		if err := g.parseFile(file); err != nil {
			return fmt.Errorf("failed to parse %s: %w", file, err)
		}
	}

	return nil
}

// ParseFile parses a single Go file
func (g *CodeGraph) ParseFile(path string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.parseFile(path)
}

func (g *CodeGraph) parseFile(path string) error {
	node, err := parser.ParseFile(g.fset, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	pkgName := node.Name.Name
	absPath, _ := filepath.Abs(path)

	pkg, ok := g.Packages[pkgName]
	if !ok {
		pkg = &PackageInfo{
			Name:       pkgName,
			Path:       filepath.Dir(absPath),
			Imports:    make([]string, 0),
			Functions:  make([]FuncInfo, 0),
			Structs:    make([]StructInfo, 0),
			Interfaces: make([]InterfaceInfo, 0),
		}
		g.Packages[pkgName] = pkg
	}

	currentPkg := pkg.Name

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ImportSpec:
			if x.Path != nil {
				importPath := strings.Trim(x.Path.Value, `"`)
				pkg.Imports = append(pkg.Imports, importPath)
			}

		case *ast.FuncDecl:
			funcInfo := g.extractFuncInfo(x, path, currentPkg)
			pkg.Functions = append(pkg.Functions, funcInfo)

			// Extract call edges
			g.extractCallEdges(x, path, currentPkg, funcInfo.Name)

		case *ast.GenDecl:
			if x.Tok == token.TYPE {
				for _, spec := range x.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						switch t := typeSpec.Type.(type) {
						case *ast.StructType:
							structInfo := g.extractStructInfo(typeSpec, t, path, currentPkg)
							pkg.Structs = append(pkg.Structs, structInfo)

						case *ast.InterfaceType:
							interfaceInfo := g.extractInterfaceInfo(typeSpec, t, path, currentPkg)
							pkg.Interfaces = append(pkg.Interfaces, interfaceInfo)
						}
					}
				}
			}
		}
		return true
	})

	return nil
}

func (g *CodeGraph) extractFuncInfo(decl *ast.FuncDecl, path, pkgName string) FuncInfo {
	pos := g.fset.Position(decl.Pos())

	doc := ""
	if decl.Doc != nil {
		doc = decl.Doc.Text()
	}

	receiver := ""
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		receiver = g.exprToString(decl.Recv.List[0].Type)
	}

	params := make([]string, 0)
	if decl.Type.Params != nil {
		for _, p := range decl.Type.Params.List {
			if len(p.Names) > 0 {
				params = append(params, fmt.Sprintf("%s %s", p.Names[0].Name, g.exprToString(p.Type)))
			} else {
				params = append(params, g.exprToString(p.Type))
			}
		}
	}

	returns := make([]string, 0)
	if decl.Type.Results != nil {
		for _, r := range decl.Type.Results.List {
			if len(r.Names) > 0 {
				returns = append(returns, fmt.Sprintf("%s %s", r.Names[0].Name, g.exprToString(r.Type)))
			} else {
				returns = append(returns, g.exprToString(r.Type))
			}
		}
	}

	return FuncInfo{
		Name:     decl.Name.Name,
		Receiver: receiver,
		Params:   params,
		Returns:  returns,
		File:     path,
		Line:     pos.Line,
		Doc:      doc,
	}
}

func (g *CodeGraph) extractStructInfo(spec *ast.TypeSpec, structType *ast.StructType, path, pkgName string) StructInfo {
	pos := g.fset.Position(spec.Pos())

	fields := make([]FieldInfo, 0)
	if structType.Fields != nil {
		for _, f := range structType.Fields.List {
			fieldNames := make([]string, 0)
			if len(f.Names) > 0 {
				for _, n := range f.Names {
					fieldNames = append(fieldNames, n.Name)
				}
			} else {
				fieldNames = append(fieldNames, "")
			}

			fieldType := g.exprToString(f.Type)
			tag := ""
			if f.Tag != nil {
				tag = strings.Trim(f.Tag.Value, "`")
			}

			for _, name := range fieldNames {
				fields = append(fields, FieldInfo{
					Name: name,
					Type: fieldType,
					Tag:  tag,
				})
			}
		}
	}

	return StructInfo{
		Name:    spec.Name.Name,
		Fields:  fields,
		Methods: make([]string, 0),
		File:    path,
		Line:    pos.Line,
	}
}

func (g *CodeGraph) extractInterfaceInfo(spec *ast.TypeSpec, ifaceType *ast.InterfaceType, path, pkgName string) InterfaceInfo {
	pos := g.fset.Position(spec.Pos())

	methods := make([]string, 0)
	if ifaceType.Methods != nil {
		for _, m := range ifaceType.Methods.List {
			switch t := m.Type.(type) {
			case *ast.FuncType:
				sig := g.funcTypeToString(t)
				if len(m.Names) > 0 {
					methods = append(methods, fmt.Sprintf("%s%s", m.Names[0].Name, sig))
				}
			case *ast.Ident:
				methods = append(methods, t.Name)
			}
		}
	}

	return InterfaceInfo{
		Name:    spec.Name.Name,
		Methods: methods,
		File:    path,
		Line:    pos.Line,
	}
}

func (g *CodeGraph) extractCallEdges(decl *ast.FuncDecl, path, pkgName, funcName string) {
	caller := fmt.Sprintf("%s.%s", pkgName, funcName)
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		receiverType := g.exprToString(decl.Recv.List[0].Type)
		// Simplify receiver type
		if strings.HasPrefix(receiverType, "*") {
			receiverType = receiverType[1:]
		}
		caller = fmt.Sprintf("%s.%s", receiverType, funcName)
	}

	ast.Inspect(decl, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if fun, ok := call.Fun.(*ast.Ident); ok {
				callee := fmt.Sprintf("%s.%s", pkgName, fun.Name)
				g.CallGraph = append(g.CallGraph, CallEdge{
					Caller: caller,
					Callee: callee,
					File:   path,
				})
			}
		}
		return true
	})
}

func (g *CodeGraph) exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + g.exprToString(t.X)
	case *ast.ArrayType:
		return "[]" + g.exprToString(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", g.exprToString(t.Key), g.exprToString(t.Value))
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", g.exprToString(t.X), t.Sel.Name)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return g.funcTypeToString(t)
	case *ast.ChanType:
		return "chan " + g.exprToString(t.Value)
	case *ast.Ellipsis:
		return "..." + g.exprToString(t.Elt)
	default:
		return "unknown"
	}
}

func (g *CodeGraph) funcTypeToString(ft *ast.FuncType) string {
	params := make([]string, 0)
	if ft.Params != nil {
		for _, p := range ft.Params.List {
			params = append(params, g.exprToString(p.Type))
		}
	}

	results := make([]string, 0)
	if ft.Results != nil {
		for _, r := range ft.Results.List {
			results = append(results, g.exprToString(r.Type))
		}
	}

	resultStr := strings.Join(results, ", ")
	if len(results) > 1 {
		resultStr = fmt.Sprintf("(%s)", resultStr)
	}

	return fmt.Sprintf("(%s) %s", strings.Join(params, ", "), resultStr)
}

// FindFunction searches for functions by name across all packages
func (g *CodeGraph) FindFunction(name string) []FuncInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()

	results := make([]FuncInfo, 0)
	for _, pkg := range g.Packages {
		for _, fn := range pkg.Functions {
			if strings.EqualFold(fn.Name, name) {
				results = append(results, fn)
			}
		}
	}
	return results
}

// FindStruct searches for structs by name across all packages
func (g *CodeGraph) FindStruct(name string) []StructInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()

	results := make([]StructInfo, 0)
	for _, pkg := range g.Packages {
		for _, st := range pkg.Structs {
			if strings.EqualFold(st.Name, name) {
				results = append(results, st)
			}
		}
	}
	return results
}

// FindCallers finds which functions call the given function
func (g *CodeGraph) FindCallers(funcName string) []CallEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	results := make([]CallEdge, 0)
	for _, edge := range g.CallGraph {
		if strings.HasSuffix(edge.Callee, "."+funcName) {
			results = append(results, edge)
		}
	}
	return results
}

// FindCallees finds which functions are called by the given function
func (g *CodeGraph) FindCallees(funcName string) []CallEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	results := make([]CallEdge, 0)
	for _, edge := range g.CallGraph {
		if strings.HasSuffix(edge.Caller, "."+funcName) {
			results = append(results, edge)
		}
	}
	return results
}

// FindImplementations finds structs that implement the given interface
func (g *CodeGraph) FindImplementations(interfaceName string) []StructInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()

	results := make([]StructInfo, 0)
	for _, pkg := range g.Packages {
		for _, st := range pkg.Structs {
			for _, implInterface := range g.Implementations[st.Name] {
				if strings.EqualFold(implInterface, interfaceName) {
					results = append(results, st)
					break
				}
			}
		}
	}
	return results
}

// GetDependencies returns imports and transitive dependencies for a file
func (g *CodeGraph) GetDependencies(file string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	deps := make(map[string]bool)
	queue := make([]string, 0)

	// Find which package this file belongs to
	for _, pkg := range g.Packages {
		for _, fn := range pkg.Functions {
			if strings.HasSuffix(fn.File, file) {
				queue = append(queue, pkg.Imports...)
				break
			}
		}
	}

	// BFS to find transitive dependencies
	visited := make(map[string]bool)
	for len(queue) > 0 {
		dep := queue[0]
		queue = queue[1:]

		if visited[dep] {
			continue
		}
		visited[dep] = true

		if strings.HasPrefix(dep, ".") || strings.HasPrefix(dep, "_") {
			// Skip relative and C dependencies
			continue
		}

		deps[dep] = true

		// Add imports from dependency package
		for _, pkg := range g.Packages {
			if pkg.Name == filepath.Base(dep) || pkg.Path == dep {
				for _, imp := range pkg.Imports {
					queue = append(queue, imp)
				}
				break
			}
		}
	}

	result := make([]string, 0, len(deps))
	for dep := range deps {
		result = append(result, dep)
	}
	return result
}

// Stats returns statistics about the code graph
func (g *CodeGraph) Stats() GraphStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	totalFuncs := 0
	totalStructs := 0
	totalInterfaces := 0

	for _, pkg := range g.Packages {
		totalFuncs += len(pkg.Functions)
		totalStructs += len(pkg.Structs)
		totalInterfaces += len(pkg.Interfaces)
	}

	return GraphStats{
		Packages:   len(g.Packages),
		Functions:  totalFuncs,
		Structs:    totalStructs,
		Interfaces: totalInterfaces,
		CallEdges:  len(g.CallGraph),
	}
}