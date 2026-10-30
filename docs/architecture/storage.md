# Storage e Banco de Dados (Postgres + Storage Provider)

Esta documentação descreve como a Suitee armazena dados e arquivos no estado atual do sistema.

## 1. Fonte de Verdade (Dados)

- **Banco principal**: Postgres (pode ser Supabase Postgres).
- **Modelagem predominante**: tabelas por domínio com colunas padronizadas (`id`, `tenant_id`, `metadata JSONB`, `created_at`, `updated_at`, `deleted_at`).
- **Isolamento multi-tenant**: aplicado via RLS (Row Level Security) quando usando Supabase Postgres e reforçado pelo middleware (claim `app_metadata.tenant_id` no JWT).

### 1.1. Conexões (DATABASE_URI vs DATABASE_URI_SERVICE)

- `DATABASE_URI`: conexão padrão usada pelas requisições tenant-scoped (RLS).
- `DATABASE_URI_SERVICE`: conexão usada para operações internas que precisam bypassar RLS (ex.: jobs, outbox, varreduras cross-tenant). Em Supabase Postgres, isso normalmente exige uma conexão direta (porta 5432) com um role com `BYPASSRLS`/`service_role`.

As variáveis e recomendações ficam em [.env.example](../../.env.example).

## 2. Arquivos (Storage)

O upload/download é centralizado no pacote `pkg/storage` e controlado por `STORAGE_PROVIDER`.

- **Produção (opcional)**: `STORAGE_PROVIDER=supabase` (Supabase Storage).
- **Dev/local (opcional)**: MinIO (S3 compatível) pode ser usado como alternativa e também serve como origem do `imgproxy` no `docker-compose.yml`.

### 2.1. Buckets e paths

- O mapeamento de buckets e regras fica em [storage_buckets.md](../supabase/storage_buckets.md).
- Convenção recomendada de path: `{tenant_id}/{dominio}/{uuid}.{ext}`.

### 2.2. Público vs privado

- Buckets públicos: leitura direta via URL.
- Buckets privados: download via URL assinada (gerada pelo backend sob demanda) e policies baseadas em `tenant_id`.

## 3. Ambiente Local

- A stack local pode usar Postgres remoto (Supabase Postgres) configurado em `DATABASE_URI` ou um Postgres local via compose.
- Para rodar um Postgres local, suba o perfil `local-db` do compose e ajuste o `DATABASE_URI` para apontar para `postgres:5432` (ver comentários em `.env.example`).
