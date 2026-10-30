# Suitee Core API

Esta é a API RESTful do projeto Suitee Core, construída em GoLang utilizando o framework Gin e Postgres (Supabase).

## Estrutura do Projeto

O projeto segue uma arquitetura limpa (Clean Architecture) com separação de responsabilidades:

- **cmd/server**: Ponto de entrada da aplicação.
- **internal/api**: Camada de apresentação (Handlers, Middlewares, Rotas).
- **internal/domain**: Definições das entidades de domínio e regras de negócio.
- **internal/repository**: Camada de acesso a dados (Postgres).
- **internal/service**: Camada de serviços (Casos de uso).
- **pkg**: Pacotes reutilizáveis (Database, Logger).

## Pré-requisitos

- Go 1.25+

## Configuração

1. Clone o repositório.
2. Copie o arquivo de exemplo de variáveis de ambiente (se houver) ou configure as variáveis necessárias. O projeto carrega configurações de variáveis de ambiente.

### Variáveis de Ambiente Principais

| Variável | Descrição | Padrão |
|----------|-----------|--------|
| PORT | Porta do servidor HTTP | 8082 |
| DATABASE_URI | URI do Postgres | ver `.env.example` na raiz |
| DATABASE_URI_SERVICE | URI para operações internas (bypass RLS) | ver `.env.example` na raiz |
| DATABASE_NAME | Schema lógico do serviço | toca_core |
| APP_ENV | Ambiente (development/production) | development |
| LOG_LEVEL | Nível de log (debug/info/warn/error) | info |
| ADDRESS_VERIFICATION_MODE | Política de falha do verificador: `best_effort`, `strict` ou `disabled` | best_effort |
| ADDRESS_VERIFICATION_PROVIDER | Provider de CEP (`viacep`) | viacep |
| ADDRESS_VERIFICATION_BASE_URL | Endpoint-base do provider | https://viacep.com.br |
| ADDRESS_VERIFICATION_TIMEOUT | Limite total por consulta | 2s |
| ADDRESS_VERIFICATION_CACHE_TTL | Validade do cache em memória para CEPs encontrados/não encontrados | 24h |
| ADDRESS_VERIFICATION_CACHE_MAX_ENTRIES | Limite de CEPs no cache por instância | 10000 |

### Verificação de endereços

Os fluxos `POST /properties` e `PUT /properties/:id` normalizam o endereço antes da persistência. Para endereços brasileiros, o CEP é reduzido aos oito dígitos canônicos e consultado no ViaCEP; rua, bairro, cidade e UF retornados pelo provider substituem os valores informados, enquanto número, complemento e ponto de referência são preservados. O campo `is_verified` enviado pelo cliente nunca é aceito como prova: ele só fica `true` após uma consulta bem-sucedida.

CEPs malformados ou definitivamente inexistentes sempre rejeitam a operação com HTTP 400. A política para indisponibilidade/timeout do provider é explícita:

| Modo | Comportamento na indisponibilidade do provider |
|------|-----------------------------------------------|
| `best_effort` | Grava somente se o endereço informado continuar completo; fica `is_verified=false` e gera warning estruturado. |
| `strict` | Não grava e responde HTTP 503. |
| `disabled` | Não chama provider; apenas normaliza/valida e grava como não verificado. |

Endereços fora do Brasil não são enviados ao ViaCEP: são normalizados, validados e mantidos como não verificados. O cache é local a cada processo, tem TTL e capacidade limitados e armazena tanto acertos quanto respostas definitivas de CEP inexistente.

## Executando Localmente

Para rodar o serviço como parte da stack local:

```bash
make run
```

Para rodar apenas este serviço (modo desenvolvimento):

```bash
# Instalar dependências
go mod tidy

# Executar a aplicação
go run cmd/server/main.go
```

O servidor iniciará em `http://localhost:8082`.

## Endpoints Disponíveis

### Health Check

- `GET /api/v1/health`: Verifica o status da API.

### Propriedades (Exemplo CRUD)

- `POST /api/v1/properties`: Cria uma nova propriedade.
- `GET /api/v1/properties`: Lista propriedades com paginação (`?limit=10&offset=0`).
- `GET /api/v1/properties/:id`: Obtém detalhes de uma propriedade.
- `PUT /api/v1/properties/:id`: Atualiza uma propriedade.
- `DELETE /api/v1/properties/:id`: Remove uma propriedade.

## Dependências

As principais dependências do projeto são:

- [Gin](https://github.com/gin-gonic/gin): Framework Web.
- [pgx](https://github.com/jackc/pgx): Driver/Pool Postgres.
- [sqlx](https://github.com/jmoiron/sqlx): Helper de SQL.
- [Validator](https://github.com/go-playground/validator): Validação de dados.
- [ViaCEP](https://viacep.com.br/): Verificação e unificação de endereços brasileiros por CEP.
