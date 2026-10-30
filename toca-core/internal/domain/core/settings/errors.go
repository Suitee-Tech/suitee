package settings

import (
	"fmt"
	"strings"
)

// InvalidPermissionMatrixError indica que o payload de PUT /permissions trouxe
// ids de permissão ou chaves de papel que não existem no catálogo canônico.
//
// MOTIVAÇÃO (achado A6 da revisão de permissões de 2026-09-19): o serviço
// DESCARTAVA em silêncio os ids desconhecidos e não validava as chaves de papel
// dentro de `Permission.Roles`. O cliente recebia 200 "permissions updated" e
// acreditava ter salvo — enquanto toggles do frontend (que tinha um catálogo
// próprio, divergente) simplesmente não tinham efeito. Erro explícito é a única
// forma de o operador perceber que a tela e o servidor discordam.
type InvalidPermissionMatrixError struct {
	UnknownPermissionIDs []string
	UnknownRoles         []string
}

func (e *InvalidPermissionMatrixError) Error() string {
	parts := make([]string, 0, 2)
	if len(e.UnknownPermissionIDs) > 0 {
		parts = append(parts, fmt.Sprintf("permissões desconhecidas: %s", strings.Join(e.UnknownPermissionIDs, ", ")))
	}
	if len(e.UnknownRoles) > 0 {
		parts = append(parts, fmt.Sprintf("papéis desconhecidos: %s", strings.Join(e.UnknownRoles, ", ")))
	}
	return "matriz de permissões inválida — " + strings.Join(parts, "; ")
}

// InvalidAccessProfileError indica que o payload trouxe perfis de acesso
// personalizados inválidos. Como a matriz, ele é erro do CLIENTE (400) com a
// lista do que não bate — nunca descarte silencioso: um perfil salvo pela
// metade apareceria na tela como se concedesse o que não concede.
type InvalidAccessProfileError struct {
	InvalidIDs           []string
	DuplicatedIDs        []string
	MissingNames         []string
	UnknownBaseRoles     []string
	UnknownPermissionIDs []string
}

func (e *InvalidAccessProfileError) Error() string {
	parts := make([]string, 0, 5)
	if len(e.InvalidIDs) > 0 {
		parts = append(parts, fmt.Sprintf("ids inválidos (precisam do prefixo reservado de perfil): %s", strings.Join(e.InvalidIDs, ", ")))
	}
	if len(e.DuplicatedIDs) > 0 {
		parts = append(parts, fmt.Sprintf("perfis repetidos: %s", strings.Join(e.DuplicatedIDs, ", ")))
	}
	if len(e.MissingNames) > 0 {
		parts = append(parts, fmt.Sprintf("perfis sem nome: %s", strings.Join(e.MissingNames, ", ")))
	}
	if len(e.UnknownBaseRoles) > 0 {
		parts = append(parts, fmt.Sprintf("perfis base desconhecidos: %s", strings.Join(e.UnknownBaseRoles, ", ")))
	}
	if len(e.UnknownPermissionIDs) > 0 {
		parts = append(parts, fmt.Sprintf("permissões desconhecidas: %s", strings.Join(e.UnknownPermissionIDs, ", ")))
	}
	return "perfis de acesso inválidos — " + strings.Join(parts, "; ")
}

// ErrAccessProfileNotFound — perfil inexistente no tenant (404 no handler).
type ErrAccessProfileNotFound struct{ ID string }

func (e *ErrAccessProfileNotFound) Error() string {
	return fmt.Sprintf("perfil de acesso não encontrado: %s", e.ID)
}
