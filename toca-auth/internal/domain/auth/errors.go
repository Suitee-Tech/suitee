package auth

import "errors"

// Erros de cadastro compartilhados entre o cliente Supabase, o serviço e a
// borda HTTP. As mensagens são internas e deliberadamente genéricas: a
// resposta ao cliente é definida na camada HTTP, sem ecoar detalhes do
// provedor nem confirmar a existência de uma conta.
var (
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrPhoneAlreadyExists = errors.New("phone already exists")
	// ErrSignUpFailed cobre a criação seguida de falha no login. O detalhe do
	// provedor fica no log do servidor; o cliente recebe um erro genérico.
	ErrSignUpFailed = errors.New("sign up failed")
)
