package supplier

import "context"

// Filter restringe a listagem usada na seleção de prestador.
type Filter struct {
	// Query busca por nome, tipo de serviço, e-mail ou telefone.
	Query string
	// ServiceType filtra por tipo de serviço exato (Elétrica, Vistoria...).
	ServiceType string
	// ActiveOnly exclui prestadores desativados.
	ActiveOnly bool
	Limit      int64
	Offset     int64
}

// Changes descreve o que a tela alterou. Campo nulo não é tocado — a tela
// edita um dado por vez e não pode zerar o que não foi informado.
type Changes struct {
	Name        *string
	ServiceType *string
	Kind        *string
	Document    *string
	Email       *string
	Phone       *string
	Notes       *string
	City        *string
	Active      *bool
}

type Repository interface {
	Create(ctx context.Context, v *Supplier) error
	GetByID(ctx context.Context, tenantID, id string) (*Supplier, error)
	Update(ctx context.Context, v *Supplier) error
	Delete(ctx context.Context, tenantID, id string) error
	List(ctx context.Context, tenantID string, filter Filter) ([]*Supplier, int64, error)
	// FindDuplicate localiza o prestador já cadastrado pelo mesmo documento ou
	// contato, para o cadastro reutilizar em vez de duplicar.
	FindDuplicate(ctx context.Context, tenantID, document, phone, email string) (*Supplier, error)
}
