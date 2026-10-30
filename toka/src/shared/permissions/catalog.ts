// Catálogo de permissões do frontend.
//
// FONTE ÚNICA (achado A6 da revisão de permissões de 2026-09-19): este arquivo
// apenas reexporta o catálogo GERADO a partir de `pkg/auth/permission_catalog.go`
// e `pkg/auth/roles.go`. Antes havia dois catálogos escritos à mão que
// divergiram em 46 de 63 permissões (sempre com o frontend mais permissivo), e
// salvar a tela de Permissões escalava privilégios por causa disso.
//
// Para mudar permissões, papéis, rótulos ou defaults: edite o Go e rode
//
//	go run ./pkg/cmd/gen-permission-catalog
//
// `make check-permission-catalog` (parte de `make check`) falha quando o arquivo
// gerado está defasado.
export * from "./catalog.generated";
