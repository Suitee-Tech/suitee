// check-internal-tenant-param trava a classe de defeito que derrubou a tela
// "Financeiro" do portal do cliente em 2026-09-19.
//
// Desde o endurecimento do achado A3, as rotas internas resolvem o tenant pela
// CREDENCIAL (`auth.ResolveTenantForInternalRequest`): chamada de serviço sem
// `tenant_id` explícito responde 400. O cliente HTTP do toca-deal chamava
// `/api/v1/financial/internal/deals/:id/charges` sem o parâmetro, o handler do
// portal traduzia o 400 em 403 e a cliente lia "acesso negado" — defeito de
// parâmetro parecendo defeito de autorização.
//
// Regra: toda função em `pkg/clients` que monta um caminho `/internal/` precisa
// carregar o tenant de forma VERIFICÁVEL na própria chamada:
//
//  1. string literal com `tenant_id` (query ou path);
//  2. corpo tipado cujo tipo declara `TenantID` (o struct vai no `Do(...)`);
//  3. parâmetro de tenant (`tenantID`/`companyID`) interpolado no caminho —
//     caso das rotas em que o próprio recurso É o tenant.
//
// Não basta a função mencionar o tenant numa guarda: aceitar isso deixaria
// passar exatamente o defeito original (remover o `tenant_id` da query mantendo
// a validação de entrada). A prova por mutação está em
// `docs/reviews/evidence/p2/incidente-portal-charges-403.md`.
//
// Usage:
//
//	go run ./scripts/check-internal-tenant-param -root ../..
//
// Exit codes: 0 ok, 1 violação, 2 erro de uso/IO.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// exemptFunctions lista funções cujo caminho interno NÃO carrega tenant_id na
// própria chamada, com o motivo.
var exemptFunctions = map[string]string{
	"PushInAppNotification": "o tenant viaja dentro de Data[\"tenant_id\"] do corpo (o destino fecha com 400 se faltar) — ver charge_service.go",
}

// structFields indexa nome do tipo → campos declarados no arquivo.
type structFields map[string]map[string]bool

type violation struct {
	File   string
	Line   int
	Func   string
	Path   string
	Detail string
}

func main() {
	root := flag.String("root", ".", "raiz do repositório")
	flag.Parse()

	clientsDir := filepath.Join(*root, "pkg", "clients")
	entries, err := os.ReadDir(clientsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-internal-tenant-param: %v\n", err)
		os.Exit(2)
	}

	var violations []violation
	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++
		found, err := scan(filepath.Join(clientsDir, name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "check-internal-tenant-param: %v\n", err)
			os.Exit(2)
		}
		violations = append(violations, found...)
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		return violations[i].Line < violations[j].Line
	})

	if len(violations) == 0 {
		fmt.Printf("check-internal-tenant-param: ok (%d arquivos em pkg/clients)\n", scanned)
		return
	}
	for _, v := range violations {
		fmt.Printf("%s:%d: %s chama %q sem tenant\n    %s\n", v.File, v.Line, v.Func, v.Path, v.Detail)
	}
	fmt.Printf("\ncheck-internal-tenant-param: %d violação(ões)\n", len(violations))
	os.Exit(1)
}

func scan(path string) ([]violation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	types := collectStructFields(file)

	var out []violation
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		paths := internalPaths(fn.Body)
		if len(paths) == 0 {
			continue
		}
		if _, exempt := exemptFunctions[fn.Name.Name]; exempt {
			continue
		}
		if tenantIsVerifiable(fn, types) {
			continue
		}
		out = append(out, violation{
			File:   filepath.ToSlash(path),
			Line:   fset.Position(fn.Pos()).Line,
			Func:   fn.Name.Name,
			Path:   paths[0],
			Detail: "carregue o tenant na própria chamada (`?tenant_id=` no caminho, corpo com TenantID ou o tenant como parâmetro de path) — a rota interna resolve o tenant pela credencial e recusa sem ele (400)",
		})
	}
	return out, nil
}

// internalPaths devolve os literais de caminho `/internal/` usados no corpo.
func internalPaths(body *ast.BlockStmt) []string {
	var paths []string
	ast.Inspect(body, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value := strings.Trim(lit.Value, "`\"")
		if strings.Contains(value, "/internal/") {
			paths = append(paths, value)
		}
		return true
	})
	return paths
}

// tenantIsVerifiable reconhece o tenant carregado de forma explícita no corpo.
func tenantIsVerifiable(fn *ast.FuncDecl, types structFields) bool {
	params := paramTypes(fn)
	carried := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if carried {
			return false
		}
		switch n := node.(type) {
		case *ast.BasicLit:
			if n.Kind == token.STRING && strings.Contains(strings.Trim(n.Value, "`\""), "tenant_id") {
				carried = true
			}
		case *ast.CallExpr:
			// fmt.Sprintf("/…/%s", companyID): o tenant é o path param.
			if isSprintf(n.Fun) {
				for _, arg := range n.Args {
					if name, ok := arg.(*ast.Ident); ok && isTenantishName(name.Name) {
						carried = true
					}
				}
			}
			// c.BaseClient.Do(ctx, method, path, req) com req.TenantID.
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel != nil && sel.Sel.Name == "Do" && len(n.Args) >= 4 {
				if name, ok := n.Args[3].(*ast.Ident); ok && types[params[name.Name]]["TenantID"] {
					carried = true
				}
			}
		}
		return true
	})
	return carried
}

// paramTypes mapeia nome do parâmetro → nome do tipo declarado.
func paramTypes(fn *ast.FuncDecl) map[string]string {
	out := map[string]string{}
	if fn.Type.Params == nil {
		return out
	}
	for _, param := range fn.Type.Params.List {
		typeName := exprTypeName(param.Type)
		for _, name := range param.Names {
			out[name.Name] = typeName
		}
	}
	return out
}

func exprTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return exprTypeName(t.X)
	}
	return ""
}

// isSprintf reconhece `Sprintf(...)` e `fmt.Sprintf(...)`.
func isSprintf(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name == "Sprintf"
	case *ast.SelectorExpr:
		return f.Sel != nil && f.Sel.Name == "Sprintf"
	}
	return false
}

func isTenantishName(name string) bool {
	lower := strings.ToLower(name)
	return lower == "tenantid" || lower == "companyid"
}

// collectStructFields indexa os campos de cada struct declarada no arquivo —
// quando o corpo tipado vai no `Do(...)`, o tenant viaja no struct.
func collectStructFields(file *ast.File) structFields {
	types := structFields{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}
			fields := map[string]bool{}
			for _, field := range structType.Fields.List {
				for _, fieldName := range field.Names {
					fields[fieldName.Name] = true
				}
			}
			types[typeSpec.Name.Name] = fields
		}
	}
	return types
}
