package repository

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"
)

// Guarda de forma das duas queries service-scope usadas pelo webhook da
// Clicksign. Roda no CI sem banco: o que causou o incidente de 30/09/2026
// (contrato 277b965e preso em PENDING_SIGNATURE com o envelope já fechado no
// provedor) foi a FORMA da query, não a lógica do handler.
func TestDocumentKeyLookupQueryIsNullSafe(t *testing.T) {
	// `metadata->'documents'` pode ser JSON `null` (3 envelopes antigos no beta).
	// A ordem de avaliação de um AND não é garantida, então um
	// `jsonb_typeof(...) = 'array' AND EXISTS (jsonb_array_elements(coluna))` AINDA
	// pode estourar com "cannot extract elements from a scalar". A proteção tem
	// de estar na expressão passada para jsonb_array_elements.
	if strings.Contains(documentKeyLookupQuery, "jsonb_array_elements(metadata->'documents')") {
		t.Error("jsonb_array_elements não pode receber metadata->'documents' cru: com JSON null a query inteira falha (planner pode avaliar o EXISTS antes de qualquer guarda)")
	}
	if strings.Contains(documentKeyLookupQuery, "COALESCE(metadata->'documents'") {
		t.Error("COALESCE(metadata->'documents', '[]') não protege contra JSON null — jsonb null não é SQL NULL; use CASE + jsonb_typeof")
	}
	if !strings.Contains(documentKeyLookupQuery, "jsonb_typeof(metadata->'documents') = 'array'") {
		t.Error("a query de document key precisa converter o que não for array para '[]' com CASE + jsonb_typeof")
	}
}

// TestEnvelopeLookupsPropagateErrors trava a segunda metade da causa raiz: os
// lookups do webhook engoliam TODO erro de Scan como "não encontrado", então a
// query quebrada virava 200 "envelope not found locally" e o evento era
// descartado em vez de reentregue.
func TestEnvelopeLookupsPropagateErrors(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "envelope_postgres.go", nil, 0)
	if err != nil {
		t.Fatalf("parse envelope_postgres.go: %v", err)
	}

	bodies := map[string]string{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var b bytes.Buffer
		if err := printer.Fprint(&b, fset, fn); err != nil {
			t.Fatalf("render %s: %v", fn.Name.Name, err)
		}
		bodies[fn.Name.Name] = b.String()
	}

	scan, ok := bodies["scanEnvelopeRow"]
	if !ok {
		t.Fatal("scanEnvelopeRow não existe: os lookups do webhook devem compartilhar o tratamento de erro")
	}
	if !strings.Contains(scan, "errors.Is(err, sql.ErrNoRows)") {
		t.Error("scanEnvelopeRow precisa tratar especificamente sql.ErrNoRows como 'não encontrado'")
	}
	if !strings.Contains(scan, "return nil, fmt.Errorf") {
		t.Error("scanEnvelopeRow precisa devolver erro de verdade em qualquer falha que não seja no-rows")
	}

	for _, name := range []string{"GetByExternalID", "GetByDocumentKey"} {
		body, ok := bodies[name]
		if !ok {
			t.Fatalf("%s não encontrada", name)
		}
		if !strings.Contains(body, "scanEnvelopeRow(") {
			t.Errorf("%s precisa delegar para scanEnvelopeRow — sem isso um `return nil, nil` volta a esconder erro de banco", name)
		}
		if strings.Contains(body, "return nil, nil") {
			t.Errorf("%s não pode devolver (nil, nil) direto: erro real tem de subir", name)
		}
	}
}

// TestServiceScopeReadsUseServicePool trava os métodos que rodam SEM JWT: a
// varredura de reconciliação existe justamente para os casos em que o webhook
// não chegou, mas pelo pool de aplicação a política de RLS de
// signature.envelopes (tenant_id = auth.jwt()) devolve zero linhas — a rede de
// segurança fica decorativa e o envelope preso em RUNNING nunca é reconciliado.
func TestServiceScopeReadsUseServicePool(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "envelope_postgres.go", nil, 0)
	if err != nil {
		t.Fatalf("parse envelope_postgres.go: %v", err)
	}
	bodies := map[string]string{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var b bytes.Buffer
		if err := printer.Fprint(&b, fset, fn); err != nil {
			t.Fatalf("render %s: %v", fn.Name.Name, err)
		}
		bodies[fn.Name.Name] = b.String()
	}

	serviceScoped := map[string]string{
		"ListStaleActive":    "QueryServiceRows",
		"GetByIDService":     "QueryServiceRow",
		"GetByExternalID":    "QueryServiceRow",
		"GetByDocumentKey":   "QueryServiceRow",
		"UpdateService":      "QueryServiceRow",
		"AppendWebhookEvent": "ExecService",
	}
	for fn, want := range serviceScoped {
		body, ok := bodies[fn]
		if !ok {
			t.Errorf("%s não encontrada", fn)
			continue
		}
		if !strings.Contains(body, want) {
			t.Errorf("%s precisa usar %s (roda sem JWT; pelo pool de app a RLS esconde as linhas)", fn, want)
		}
	}
}
