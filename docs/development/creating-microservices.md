# Criando Novos Microsserviços na Suitee

Este guia detalha o processo para criar um novo microsserviço no ecossistema da plataforma Suitee.

## 1. Estrutura Padrão (Clean Architecture)

Todo microsserviço deve ser criado em uma nova pasta na raiz do projeto (ex: `toca-marketing/`) e seguir a estrutura padrão baseada em Domain-Driven Design (DDD) e Clean Architecture:

```text
toca-marketing/
├── cmd/
│   └── server/
│       └── main.go           # Ponto de entrada da aplicação
├── internal/
│   ├── api/
│   │   ├── handlers/         # Controladores HTTP (Gin)
│   │   ├── middlewares/      # Middlewares locais (se aplicável)
│   │   └── routes/           # Definição das rotas
│   ├── domain/               # Entidades de negócio e Interfaces
│   ├── repository/           # Implementação de acesso a dados (Postgres/JSONB)
│   └── service/              # Casos de uso (Regras de negócio)
├── Dockerfile                # Arquivo para containerização
├── go.mod                    # Gerenciador de dependências Go
└── README.md                 # Documentação local do serviço
```

## 2. Passo a Passo

### 2.1. Inicialização do Módulo Go
```bash
mkdir toca-marketing
cd toca-marketing
go mod init github.com/toca/toca-marketing
```

### 2.2. Porta e Rotas no Gateway
Para que o frontend consiga acessar o seu serviço, ele deve estar mapeado no **Gateway**.
1. Defina uma porta interna única (ex: `:8091`).
2. Adicione a rota no arquivo `gateway/main.go`:
   ```go
   // Exemplo no Gateway
   router.Any("/api/v1/marketing/*path", proxyMiddleware("http://toca-marketing:8091"))
   ```

### 2.3. Dependências Comuns
Geralmente você precisará dos seguintes pacotes no `go.mod`:
- `github.com/gin-gonic/gin` (Framework Web)
- `github.com/jackc/pgx/v5` + `github.com/jmoiron/sqlx` (Postgres)
- Pacotes internos: adicione ao `go.work` na raiz do projeto para desenvolvimento local.

### 2.4. Integração com Docker Compose
Adicione o novo serviço aos arquivos `docker-compose.yml` (local) e `docker-compose.prod.yml` (produção):

```yaml
  toca-marketing:
    build:
      context: ./toca-marketing
    container_name: toca-marketing
    environment:
      - PORT=8091
      - DATABASE_URI=${DATABASE_URI}
      - SUPABASE_URL=${SUPABASE_URL}
      - SUPABASE_ANON_KEY=${SUPABASE_ANON_KEY}
      - SUPABASE_SERVICE_ROLE_KEY=${SUPABASE_SERVICE_ROLE_KEY}
      - INTERNAL_API_KEY=${INTERNAL_API_KEY}
    networks:
      - toca-network
```

### 2.5. Integração com GitHub Actions (CI/CD)
Edite o arquivo `.github/workflows/deploy-hostinger.yml` para adicionar o novo serviço na matriz de build:
```yaml
strategy:
  matrix:
    service: [gateway, toca-auth, toca-core, toca-marketing, ...]
```

## 3. Boas Práticas

- **Isolamento de Banco de Dados**: seu serviço deve persistir seus dados no schema/tabelas do seu domínio (ex.: `marketing.*`) e respeitar o isolamento por `tenant_id` (RLS quando usando Supabase Postgres). Evite acessar tabelas de outros domínios diretamente.
- **Comunicação entre serviços**: prefira eventos via **RabbitMQ** para integrações entre domínios; use HTTP interno (clientes tipados em `pkg/clients`) apenas quando houver necessidade real de resposta síncrona.
- **Autenticação**: utilize o middleware de validação de JWT (JWKS do Supabase Auth) para proteger as rotas do seu serviço e obter `tenant_id` via claims.
