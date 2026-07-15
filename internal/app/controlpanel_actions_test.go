package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestEveryControlPanelActionReachesApplicationHandling(t *testing.T) {
	defined := declaredControlPanelActions(t, filepath.Join("..", "ui", "controlpanel", "types.go"))
	referenced := referencedControlPanelActions(t, "controlpanel_actions.go")
	var missing []string
	for action := range defined {
		if !referenced[action] {
			missing = append(missing, action)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("control-panel actions missing from application handling: %s", strings.Join(missing, ", "))
	}
}

func declaredControlPanelActions(t *testing.T, path string) map[string]bool {
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

func referencedControlPanelActions(t *testing.T, path string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || !strings.HasPrefix(selector.Sel.Name, "Act") {
			return true
		}
		packageName, ok := selector.X.(*ast.Ident)
		if ok && packageName.Name == "controlpanel" {
			actions[selector.Sel.Name] = true
		}
		return true
	})
	return actions
}
