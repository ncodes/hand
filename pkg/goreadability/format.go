package goreadability

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

type FormatOptions struct {
	Write            bool
	IncludeGenerated bool
}

type FormatResult struct {
	Files   int
	Changed []string
}

func FormatPaths(paths []string, options FormatOptions) (FormatResult, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	files, err := collectGoFiles(paths)
	if err != nil {
		return FormatResult{}, err
	}

	result := FormatResult{Files: len(files)}
	for _, path := range files {
		changed, err := formatFile(path, options)
		if err != nil {
			return FormatResult{}, err
		}

		if changed {
			result.Changed = append(result.Changed, path)
		}
	}

	return result, nil
}

func formatFile(path string, options FormatOptions) (bool, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}

	formatted, generated, err := formatSource(path, source)
	if err != nil {
		return false, err
	}
	if generated && !options.IncludeGenerated {
		return false, nil
	}
	if bytes.Equal(source, formatted) {
		return false, nil
	}

	if options.Write {
		info, err := os.Stat(path)
		if err != nil {
			return false, fmt.Errorf("stat %s: %w", path, err)
		}
		if err := os.WriteFile(path, formatted, info.Mode().Perm()); err != nil {
			return false, fmt.Errorf("write %s: %w", path, err)
		}
	}

	return true, nil
}

func formatSource(filename string, source []byte) ([]byte, bool, error) {
	canonical, err := format.Source(source)
	if err != nil {
		return nil, false, fmt.Errorf("format %s: %w", filename, err)
	}

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, filename, canonical, parser.ParseComments)
	if err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", filename, err)
	}
	if ast.IsGenerated(file) {
		return canonical, true, nil
	}

	spaced := addSemanticSpacing(fileSet, file, canonical)
	formatted, err := format.Source(spaced)
	if err != nil {
		return nil, false, fmt.Errorf("format spaced %s: %w", filename, err)
	}

	return formatted, false, nil
}

func addSemanticSpacing(fileSet *token.FileSet, file *ast.File, source []byte) []byte {
	offsets := make(map[int]struct{})
	ast.Inspect(file, func(node ast.Node) bool {
		block, ok := node.(*ast.BlockStmt)
		if !ok {
			return true
		}

		for index := 1; index < len(block.List); index++ {
			previous := block.List[index-1]
			current := block.List[index]
			if !shouldSeparateStatements(block.List, index) {
				continue
			}

			start := fileSet.PositionFor(previous.End(), false).Offset
			end := fileSet.PositionFor(current.Pos(), false).Offset
			if !isSingleWhitespaceLine(source[start:end]) {
				continue
			}

			offsets[end-lenLeadingIndent(source[start:end])] = struct{}{}
		}

		return true
	})

	if len(offsets) == 0 {
		return source
	}

	ordered := make([]int, 0, len(offsets))
	for offset := range offsets {
		ordered = append(ordered, offset)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(ordered)))

	result := append([]byte(nil), source...)
	for _, offset := range ordered {
		result = slices.Insert(result, offset, '\n')
	}

	return result
}

func shouldSeparateStatements(statements []ast.Stmt, index int) bool {
	previous := statements[index-1]
	current := statements[index]
	if isLeadingGuardBoundary(statements, index) {
		return true
	}
	if index >= 2 && isLockCall(statements[index-2]) && isUnlockDefer(previous) {
		return true
	}

	return isControlBlock(previous) && isReturnStatement(current)
}

func isLeadingGuardBoundary(statements []ast.Stmt, index int) bool {
	if !isGuardClause(statements[index-1]) || isGuardClause(statements[index]) {
		return false
	}

	for _, statement := range statements[:index-1] {
		if !isGuardClause(statement) {
			return false
		}
	}

	return true
}

func isGuardClause(statement ast.Stmt) bool {
	conditional, ok := statement.(*ast.IfStmt)
	if !ok || conditional.Else != nil || len(conditional.Body.List) == 0 {
		return false
	}

	return isTerminatingStatement(conditional.Body.List[len(conditional.Body.List)-1])
}

func isTerminatingStatement(statement ast.Stmt) bool {
	switch value := statement.(type) {
	case *ast.ReturnStmt, *ast.BranchStmt:
		return true
	case *ast.ExprStmt:
		call, ok := value.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		name, ok := call.Fun.(*ast.Ident)
		return ok && name.Name == "panic"
	default:
		return false
	}
}

func isLockCall(statement ast.Stmt) bool {
	expression, ok := statement.(*ast.ExprStmt)
	return ok && isSelectorCall(expression.X, "Lock", "RLock")
}

func isUnlockDefer(statement ast.Stmt) bool {
	deferred, ok := statement.(*ast.DeferStmt)
	return ok && isSelectorCall(deferred.Call, "Unlock", "RUnlock")
}

func isSelectorCall(expression ast.Expr, names ...string) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return false
	}

	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && slices.Contains(names, selector.Sel.Name)
}

func isControlBlock(statement ast.Stmt) bool {
	switch statement.(type) {
	case *ast.ForStmt, *ast.RangeStmt, *ast.SelectStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt:
		return true
	default:
		return false
	}
}

func isReturnStatement(statement ast.Stmt) bool {
	_, ok := statement.(*ast.ReturnStmt)
	return ok
}

func isSingleWhitespaceLine(gap []byte) bool {
	return len(bytes.TrimSpace(gap)) == 0 && bytes.Count(gap, []byte{'\n'}) == 1
}

func lenLeadingIndent(gap []byte) int {
	newline := bytes.LastIndexByte(gap, '\n')
	if newline < 0 {
		return 0
	}

	return len(gap) - newline - 1
}

func collectGoFiles(paths []string) ([]string, error) {
	seen := make(map[string]struct{})
	for _, rawPath := range paths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			return nil, errors.New("codebase path is required")
		}

		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect %s: %w", path, err)
		}
		if !info.IsDir() {
			if isGoSourceFile(path) {
				seen[filepath.Clean(path)] = struct{}{}
			}
			continue
		}

		err = filepath.WalkDir(path, func(candidate string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if candidate != path && isExcludedDirectory(entry.Name()) {
					return filepath.SkipDir
				}

				return nil
			}

			if isGoSourceFile(candidate) {
				seen[filepath.Clean(candidate)] = struct{}{}
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", path, err)
		}
	}

	files := make([]string, 0, len(seen))
	for path := range seen {
		files = append(files, path)
	}

	sort.Strings(files)

	return files, nil
}

func isExcludedDirectory(name string) bool {
	return strings.HasPrefix(name, ".") || slices.Contains(
		[]string{"build", "node_modules", "testdata", "vendor"},
		name,
	)
}

func isGoSourceFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".go") && !strings.HasSuffix(base, ".pb.go")
}
