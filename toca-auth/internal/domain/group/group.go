package group

import (
	"time"

	"github.com/oginoino/toca/pkg/validation"
)

var validate = validation.New()

type Group struct {
	ID          string        `bson:"_id,omitempty" json:"id"`
	CompanyID   string        `bson:"company_id" json:"company_id"`
	Name        string        `bson:"name" json:"name"`
	Description string        `bson:"description" json:"description"`
	RoleID      string        `bson:"role_id" json:"role_id"` // papel do catálogo (pkg/auth)
	Members     []GroupMember `bson:"members" json:"members"`
	CreatedBy   string        `bson:"created_by" json:"created_by"`
	CreatedAt   time.Time     `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time     `bson:"updated_at" json:"updated_at"`

	// IsDefault — P2/M6: grupo criado pelo sistema (os nove papéis do catálogo).
	// Antes, "é default" era inferido de `RoleID ∈ papéis do catálogo`; agora que
	// grupos CUSTOM também apontam para um papel do catálogo (é isso que os torna
	// úteis), essa inferência protegeria um grupo custom de ser renomeado/apagado.
	// O campo é explícito; `CreatedBy == "system"` continua valendo como marcador
	// para os grupos gravados antes desta mudança.
	IsDefault bool `bson:"is_default,omitempty" json:"is_default,omitempty"`
}

// IsSystemDefault informa se o grupo é um dos nove grupos de papel criados
// automaticamente (compatível com dados antigos, que não têm o campo).
func (g Group) IsSystemDefault() bool {
	return g.IsDefault || g.CreatedBy == "system"
}

type GroupMember struct {
	AccountID string    `bson:"account_id" json:"account_id"`
	AddedAt   time.Time `bson:"added_at" json:"added_at"`
	AddedBy   string    `bson:"added_by" json:"added_by"`
	Name      string    `bson:"-" json:"name"`
	AvatarURL string    `bson:"-" json:"avatar_url"`
}

func (g *Group) Validate() error {
	return validate.Struct(g)
}

type CreateGroupRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
	// FunctionalRole — P2/M6: o grupo PRECISA declarar um papel do catálogo.
	// Antes o `RoleID` era `"custom_"+uuid`, um identificador que não existe em
	// catálogo nenhum (pkg/auth) e que, ao ser gravado como `functional_role` do
	// membro por AddMember/MoveMember, deixava o usuário com um papel inexistente
	// — sem permissão na matriz e sem casar com nenhum grupo de verdade.
	// Vazio cai no default `corretor` (mesmo default de AddUserToCompany), para
	// não quebrar o cliente que ainda não envia o campo.
	FunctionalRole string `json:"functional_role"`
}

func (r *CreateGroupRequest) Validate() error {
	return validate.Struct(r)
}

type UpdateGroupRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
	// FunctionalRole permite REPARAR grupos legados (`custom_<uuid>`) apontando-os
	// para um papel do catálogo. Opcional: vazio não mexe.
	FunctionalRole string `json:"functional_role"`
}

func (r *UpdateGroupRequest) Validate() error {
	return validate.Struct(r)
}
