package supplier

import "errors"

var (
	ErrInvalidPhone        = errors.New("informe um telefone válido com DDD")
	ErrInvalidEmail        = errors.New("informe um e-mail válido")
	ErrInvalidDocument     = errors.New("informe um CPF ou CNPJ válido")
	ErrNameRequired        = errors.New("nome do profissional é obrigatório")
	ErrServiceTypeRequired = errors.New("tipo de serviço é obrigatório")
	ErrContactRequired     = errors.New("informe telefone ou e-mail de contato")
	ErrInvalidKind         = errors.New("tipo de profissional deve ser externo ou interno")
	ErrNotFound            = errors.New("profissional não encontrado")
)
