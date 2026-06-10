package codegraph

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ChangeImpact represents the impact of a code change
type ChangeImpact struct {
	TargetFile    string         `json:"targetFile"`
	TargetFunc    string         `json:"targetFunc"`
	AffectedFiles []AffectedFile `json:"affectedFiles"`
	RiskLevel     string         `json:"riskLevel"` // LOW, MEDIUM, HIGH, CRITICAL
	Description   string         `json:"description"`
}

// AffectedFile represents a file affected by a change
type AffectedFile struct {
	Path     string   `json:"path"`
	Reason   string   `json:"reason"`
	Services []string `json:"services"` // which microservices affected
}

// ImpactAnalyzer combines AST graph and LSP references for impact analysis
type ImpactAnalyzer struct {
	graph *CodeGraph
	lsp   *LSPClient
	mu    sync.RWMutex
}

// NewImpactAnalyzer creates a new impact analyzer
func NewImpactAnalyzer(graph *CodeGraph, lsp *LSPClient) *ImpactAnalyzer {
	return &ImpactAnalyzer{
		graph: graph,
		lsp:   lsp,
	}
}

// AnalyzeChange performs production-grade impact analysis combining AST graph + LSP references
func (a *ImpactAnalyzer) AnalyzeChange(ctx context.Context, file, funcName string) (*ChangeImpact, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Step 1: Find function in AST graph
	targetFunc, err := a.findFunctionInGraph(file, funcName)
	if err != nil {
		return nil, fmt.Errorf("failed to find function in graph: %w", err)
	}

	// Step 2: Find all callers (who calls this func)
	callers := a.findCallers(targetFunc)

	// Step 3: Find all callees (what does this func call)
	callees := a.findCallees(targetFunc)

	// Step 4: If LSP available, get references for cross-package impact
	lspReferences := make([]Location, 0)
	if a.lsp != nil && a.lsp.IsAvailable() && a.lsp.IsInitialized() {
		references, err := a.getLSPReferences(ctx, file, targetFunc.Line, 1)
		if err == nil {
			lspReferences = references
		}
	}

	// Step 5: Trace dependency graph to find affected services
	services := a.traceDependencies(targetFunc)

	// Step 6: Compute risk level
	riskLevel := a.computeRiskLevel(len(callers), len(callees), len(lspReferences), targetFunc)

	// Step 7: Build affected files list
	affectedFiles := a.buildAffectedFiles(callers, callees, lspReferences, services)

	// Generate description
	description := a.generateDescription(targetFunc, len(callers), len(callees), len(lspReferences))

	return &ChangeImpact{
		TargetFile:    file,
		TargetFunc:    funcName,
		AffectedFiles: affectedFiles,
		RiskLevel:     riskLevel,
		Description:   description,
	}, nil
}

// findFunctionInGraph finds a function in the AST graph
func (a *ImpactAnalyzer) findFunctionInGraph(file, funcName string) (FuncInfo, error) {
	// Normalize file path
	absFile, err := filepath.Abs(file)
	if err != nil {
		return FuncInfo{}, err
	}

	// Search all packages for the function
	a.graph.mu.RLock()
	defer a.graph.mu.RUnlock()

	for _, pkg := range a.graph.Packages {
		for _, fn := range pkg.Functions {
			fnAbsFile, _ := filepath.Abs(fn.File)
			if strings.EqualFold(fnAbsFile, absFile) && strings.EqualFold(fn.Name, funcName) {
				return fn, nil
			}
		}
	}

	return FuncInfo{}, fmt.Errorf("function %s not found in file %s", funcName, file)
}

// findCallers finds all functions that call the target function
func (a *ImpactAnalyzer) findCallers(targetFunc FuncInfo) []CallEdge {
	// Build caller identifier: package.func or receiver.func
	callerID := a.buildCallerID(targetFunc)

	a.graph.mu.RLock()
	defer a.graph.mu.RUnlock()

	// Find all edges where Callee matches our target
	callers := make([]CallEdge, 0)
	for _, edge := range a.graph.CallGraph {
		if edge.Callee == callerID {
			callers = append(callers, edge)
		}
		// Also try partial match for function name only
		if strings.HasSuffix(edge.Callee, "."+targetFunc.Name) {
			callers = append(callers, edge)
		}
	}

	return callers
}

// findCallees finds all functions called by the target function
func (a *ImpactAnalyzer) findCallees(targetFunc FuncInfo) []CallEdge {
	callerID := a.buildCallerID(targetFunc)

	a.graph.mu.RLock()
	defer a.graph.mu.RUnlock()

	callees := make([]CallEdge, 0)
	for _, edge := range a.graph.CallGraph {
		if edge.Caller == callerID {
			callees = append(callees, edge)
		}
	}

	return callees
}

// buildCallerID builds a consistent caller identifier from FuncInfo
func (a *ImpactAnalyzer) buildCallerID(fn FuncInfo) string {
	if fn.Receiver != "" {
		// Method: receiver.FuncName
		receiver := strings.TrimPrefix(fn.Receiver, "*")
		return receiver + "." + fn.Name
	}

	// Find package name from file
	pkgName := ""
	a.graph.mu.RLock()
	for _, pkg := range a.graph.Packages {
		pkgAbsPath, _ := filepath.Abs(pkg.Path)
		fnAbsPath, _ := filepath.Abs(fn.File)
		if strings.HasPrefix(fnAbsPath, pkgAbsPath) {
			pkgName = pkg.Name
			break
		}
	}
	a.graph.mu.RUnlock()

	if pkgName != "" {
		return pkgName + "." + fn.Name
	}

	return fn.Name
}

// getLSPReferences gets cross-package references using LSP
func (a *ImpactAnalyzer) getLSPReferences(ctx context.Context, file string, line, col int) ([]Location, error) {
	if a.lsp == nil || !a.lsp.IsAvailable() {
		return nil, ErrLSPNotAvailable
	}

	refs, err := a.lsp.References(ctx, file, line, col)
	if err != nil {
		return nil, err
	}

	locations := make([]Location, 0, len(refs))
	for _, ref := range refs {
		locations = append(locations, ref.Location)
	}

	return locations, nil
}

// traceDependencies traces the dependency graph to find affected services
func (a *ImpactAnalyzer) traceDependencies(targetFunc FuncInfo) []string {
	// For this implementation, we'll infer services from package structure
	// In a real microservice architecture, this would use service discovery

	services := make(map[string]bool)

	// Check if function is in a proto or gRPC file
	if strings.Contains(strings.ToLower(targetFunc.File), "grpc") ||
		strings.Contains(strings.ToLower(targetFunc.File), "proto") {
		services["gRPC"] = true
		services["Proto Definitions"] = true
	}

	// Check if function implements an interface
	a.graph.mu.RLock()
	for _, pkg := range a.graph.Packages {
		for _, iface := range pkg.Interfaces {
			for _, method := range iface.Methods {
				if strings.Contains(method, targetFunc.Name) {
					services["Interface Implementations"] = true
					break
				}
			}
		}
	}
	a.graph.mu.RUnlock()

	// Add service based on directory structure
	dir := filepath.Dir(targetFunc.File)
	if strings.Contains(dir, "cmd") || strings.Contains(dir, "server") {
		services["Server"] = true
	}
	if strings.Contains(dir, "api") {
		services["API Layer"] = true
	}
	if strings.Contains(dir, "service") {
		services["Business Logic"] = true
	}
	if strings.Contains(dir, "repository") || strings.Contains(dir, "storage") {
		services["Data Layer"] = true
	}

	result := make([]string, 0, len(services))
	for svc := range services {
		result = append(result, svc)
	}
	sort.Strings(result)
	return result
}

// computeRiskLevel computes the risk level based on impact metrics
func (a *ImpactAnalyzer) computeRiskLevel(numCallers, numCallees, numLSPRefs int, targetFunc FuncInfo) string {
	totalAffected := numCallers + numCallees + numLSPRefs

	// Check for critical indicators
	isProto := strings.Contains(strings.ToLower(targetFunc.File), "proto") ||
		strings.Contains(strings.ToLower(targetFunc.File), "grpc")
	isInterface := targetFunc.Receiver != "" // methods on structs often implement interfaces

	if totalAffected > 10 || isProto {
		return "CRITICAL"
	}

	if totalAffected > 5 || isInterface {
		return "HIGH"
	}

	if totalAffected >= 2 && totalAffected <= 5 {
		return "MEDIUM"
	}

	return "LOW"
}

// buildAffectedFiles builds the list of affected files
func (a *ImpactAnalyzer) buildAffectedFiles(callers, callees []CallEdge, lspRefs []Location, services []string) []AffectedFile {
	fileMap := make(map[string]*AffectedFile)

	// Add files from callers
	for _, caller := range callers {
		if _, exists := fileMap[caller.File]; !exists {
			fileMap[caller.File] = &AffectedFile{
				Path:     caller.File,
				Reason:   fmt.Sprintf("Calls target function (%s)", caller.Caller),
				Services: services,
			}
		}
	}

	// Add files from callees
	for _, callee := range callees {
		if _, exists := fileMap[callee.File]; !exists {
			fileMap[callee.File] = &AffectedFile{
				Path:     callee.File,
				Reason:   fmt.Sprintf("Called by target function (%s)", callee.Callee),
				Services: services,
			}
		} else {
			// Append to existing reason
			fileMap[callee.File].Reason += fmt.Sprintf("; calls %s", callee.Callee)
		}
	}

	// Add files from LSP references
	for _, ref := range lspRefs {
		if _, exists := fileMap[ref.File]; !exists {
			fileMap[ref.File] = &AffectedFile{
				Path:     ref.File,
				Reason:   "Cross-package reference detected via LSP",
				Services: services,
			}
		}
	}

	// Convert to sorted slice
	result := make([]AffectedFile, 0, len(fileMap))
	for _, af := range fileMap {
		result = append(result, *af)
	}

	// Sort by path
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})

	return result
}

// generateDescription generates a human-readable description of the impact
func (a *ImpactAnalyzer) generateDescription(targetFunc FuncInfo, numCallers, numCallees, numLSPRefs int) string {
	var desc strings.Builder

	desc.WriteString(fmt.Sprintf("Analyzing impact of changes to function '%s' in %s\n", targetFunc.Name, filepath.Base(targetFunc.File)))
	desc.WriteString(fmt.Sprintf("Function signature: %s(%s)", targetFunc.Name, strings.Join(targetFunc.Params, ", ")))

	if len(targetFunc.Returns) > 0 {
		returns := strings.Join(targetFunc.Returns, ", ")
		if len(targetFunc.Returns) > 1 {
			returns = "(" + returns + ")"
		}
		desc.WriteString(fmt.Sprintf(" %s", returns))
	}
	desc.WriteString("\n\n")

	desc.WriteString(fmt.Sprintf("Direct impact:\n"))
	desc.WriteString(fmt.Sprintf("  - Called by %d function(s)\n", numCallers))
	desc.WriteString(fmt.Sprintf("  - Calls %d other function(s)\n", numCallees))
	if numLSPRefs > 0 {
		desc.WriteString(fmt.Sprintf("  - %d cross-package reference(s) detected via LSP\n", numLSPRefs))
	}

	return desc.String()
}

// GenerateReport generates a formatted report for AI consumption
func (a *ImpactAnalyzer) GenerateReport(impact *ChangeImpact) string {
	var report strings.Builder

	report.WriteString("=== CHANGE IMPACT REPORT ===\n\n")
	report.WriteString(impact.Description)

	report.WriteString(fmt.Sprintf("\nRisk Level: %s\n\n", impact.RiskLevel))

	report.WriteString("Affected Services:\n")
	if len(impact.AffectedFiles) > 0 && len(impact.AffectedFiles[0].Services) > 0 {
		for _, svc := range impact.AffectedFiles[0].Services {
			report.WriteString(fmt.Sprintf("  - %s\n", svc))
		}
	} else {
		report.WriteString("  - None identified\n")
	}

	report.WriteString(fmt.Sprintf("\nAffected Files (%d total):\n", len(impact.AffectedFiles)))
	for i, af := range impact.AffectedFiles {
		report.WriteString(fmt.Sprintf("\n%d. %s\n", i+1, af.Path))
		report.WriteString(fmt.Sprintf("   Reason: %s\n", af.Reason))
		if len(af.Services) > 0 {
			report.WriteString(fmt.Sprintf("   Services: %s\n", strings.Join(af.Services, ", ")))
		}
	}

	return report.String()
}

// SuggestTests suggests which test files to run based on impact
func (a *ImpactAnalyzer) SuggestTests(impact *ChangeImpact) []string {
	tests := make(map[string]bool)

	// Add test file for the target file
	targetTestFile := strings.TrimSuffix(impact.TargetFile, ".go") + "_test.go"
	tests[targetTestFile] = true

	// Add test files for each affected file
	for _, af := range impact.AffectedFiles {
		testFile := strings.TrimSuffix(af.Path, ".go") + "_test.go"
		tests[testFile] = true

		// Also suggest package-level tests
		dir := filepath.Dir(af.Path)
		pkgTestFile := filepath.Join(dir, "package_test.go")
		tests[pkgTestFile] = true
	}

	// Add integration tests if risk is high
	if impact.RiskLevel == "HIGH" || impact.RiskLevel == "CRITICAL" {
		// Look for integration test directories
		projectRoot := filepath.Dir(filepath.Dir(impact.TargetFile))
		integrationTests := filepath.Join(projectRoot, "integration", "*_test.go")
		tests[integrationTests] = true
	}

	// Convert to sorted slice
	result := make([]string, 0, len(tests))
	for test := range tests {
		result = append(result, test)
	}
	sort.Strings(result)

	return result
}

// AnalyzePackageImpact analyzes impact of changes to an entire package
func (a *ImpactAnalyzer) AnalyzePackageImpact(ctx context.Context, pkgPath string) (*ChangeImpact, error) {
	// Find package in graph
	var targetPkg *PackageInfo
	a.graph.mu.RLock()
	for _, pkg := range a.graph.Packages {
		if pkg.Path == pkgPath || strings.HasSuffix(pkg.Path, pkgPath) {
			targetPkg = pkg
			break
		}
	}
	a.graph.mu.RUnlock()

	if targetPkg == nil {
		return nil, fmt.Errorf("package %s not found in graph", pkgPath)
	}

	// Aggregate all files in the package
	allFiles := make(map[string]bool)
	for _, fn := range targetPkg.Functions {
		allFiles[fn.File] = true
	}

	// Build affected files with service info
	services := a.traceDependenciesForPackage(targetPkg)
	affectedFiles := make([]AffectedFile, 0, len(allFiles))
	for file := range allFiles {
		affectedFiles = append(affectedFiles, AffectedFile{
			Path:     file,
			Reason:   "Part of modified package",
			Services: services,
		})
	}

	// Compute risk level based on package size
	riskLevel := "MEDIUM"
	if len(targetPkg.Functions) > 20 {
		riskLevel = "HIGH"
	}
	if len(targetPkg.Functions) > 50 {
		riskLevel = "CRITICAL"
	}
	if len(targetPkg.Functions) < 5 {
		riskLevel = "LOW"
	}

	description := fmt.Sprintf("Analyzing impact of changes to package '%s' (%d functions)\n",
		targetPkg.Name, len(targetPkg.Functions))
	description += fmt.Sprintf("Package path: %s\n\n", targetPkg.Path)
	description += fmt.Sprintf("Exported functions: %d\n", countExportedFunctions(targetPkg))
	description += fmt.Sprintf("Structs: %d\n", len(targetPkg.Structs))
	description += fmt.Sprintf("Interfaces: %d\n", len(targetPkg.Interfaces))

	return &ChangeImpact{
		TargetFile:    targetPkg.Path,
		TargetFunc:    targetPkg.Name,
		AffectedFiles: affectedFiles,
		RiskLevel:     riskLevel,
		Description:   description,
	}, nil
}

// traceDependenciesForPackage traces dependencies for a package
func (a *ImpactAnalyzer) traceDependenciesForPackage(pkg *PackageInfo) []string {
	services := make(map[string]bool)

	// Check for gRPC/proto
	for _, fn := range pkg.Functions {
		if strings.Contains(strings.ToLower(fn.File), "grpc") ||
			strings.Contains(strings.ToLower(fn.File), "proto") {
			services["gRPC"] = true
			services["Proto Definitions"] = true
		}
	}

	// Check for interfaces
	if len(pkg.Interfaces) > 0 {
		services["Interface Definitions"] = true
	}

	// Add service based on directory structure
	for _, fn := range pkg.Functions {
		dir := filepath.Dir(fn.File)
		if strings.Contains(dir, "cmd") || strings.Contains(dir, "server") {
			services["Server"] = true
		}
		if strings.Contains(dir, "api") {
			services["API Layer"] = true
		}
		if strings.Contains(dir, "service") {
			services["Business Logic"] = true
		}
		if strings.Contains(dir, "repository") || strings.Contains(dir, "storage") {
			services["Data Layer"] = true
		}
	}

	result := make([]string, 0, len(services))
	for svc := range services {
		result = append(result, svc)
	}
	sort.Strings(result)
	return result
}

// countExportedFunctions counts exported functions in a package
func countExportedFunctions(pkg *PackageInfo) int {
	count := 0
	for _, fn := range pkg.Functions {
		if len(fn.Name) > 0 && fn.Name[0] >= 'A' && fn.Name[0] <= 'Z' {
			count++
		}
	}
	return count
}