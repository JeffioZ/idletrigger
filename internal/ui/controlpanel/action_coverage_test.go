package controlpanel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestEveryActionIsReferencedByControlPanelImplementation(t *testing.T) {
	defined := declaredActions(t, "types.go")
	referenced := map[string]bool{}
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || path == "types.go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && defined[identifier.Name] {
				referenced[identifier.Name] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var missing []string
	for action := range defined {
		if !referenced[action] {
			missing = append(missing, action)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("actions missing from control-panel implementation: %s", strings.Join(missing, ", "))
	}
}

func declaredActions(t *testing.T, path string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, specification := range general.Specs {
			values, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range values.Names {
				if strings.HasPrefix(name.Name, "Act") {
					actions[name.Name] = true
				}
			}
		}
	}
	return actions
}
