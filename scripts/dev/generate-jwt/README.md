# generate-jwt

Utilitário de desenvolvimento para gerar um JWT assinado com claims fixas — útil para testes locais contra os serviços que exigem `Authorization: Bearer ...`.

## Uso

```bash
cd scripts/dev/generate-jwt
go run .
```

O token é impresso em stdout.

## Claims padrão

```go
account_id: "69c813b937074cc36e9fe428"
tenant_id:  "69c813f937074cc36e9fe42b"
email:      "maria@email.com"
role:       "admin"
exp:        now + 1h
```

## Chave de assinatura

`changeme_in_production` (HS256). **Apenas para dev local.** Em produção, a chave é definida por `JWT_SECRET` no serviço `toca-auth`.

Para gerar tokens com outras claims, edite `main.go` — este é um helper simples, não uma CLI completa.
