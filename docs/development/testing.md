# Testes e Validação na Suitee Platform

**Estado revalidado em 03/10/2026:** a rodada consolidada dos 14 módulos Go
passou em testes e lint; **237 arquivos e 1.749 testes Vitest** passam, assim como
typecheck, fronteiras de features, build Vite e budget do bundle. O checker de
RLS passa sobre 61 migrations: 62 tabelas multi-tenant cobertas, duas exceções
service-only e uma tabela removida.

A plataforma adota uma abordagem de testes focada em unitários (backend e frontend) e E2E (Playwright).

## 1. Testes Unitários no Backend (Go)
A estratégia idiomática de Go é utilizada, mantendo arquivos de teste (`*_test.go`) na mesma pasta do código.
Para rodar os testes de um microsserviço:
```bash
make test-go MODULE=toca-financial
make test-go MODULE=toca-deal
make test-go MODULE=toca-crm
make test-go MODULE=pkg
```

## 2. Testes Unitários no Frontend (Toka)
O frontend utiliza **Vitest** como framework de testes.
```bash
make frontend-test
```

## 3. Testes End-to-End (E2E) com Playwright
O Playwright é configurado para rodar os testes E2E simulando as interações de usuários reais através da interface e APIs.
Os testes podem ser executados através do `Makefile` na raiz do projeto:
```bash
# Rodar as jornadas determinísticas usadas pelo CI
make test-e2e

# Validar a composição integrada com Postgres local e teardown garantido
make test-stack
```

O workflow de PR foi configurado para usar `toka/playwright.ci.config.ts`: constrói o artefato,
serve-o com Vite Preview e executa 174 testes em 11 specs de login, recuperação,
cadastro, onboarding, dashboard, CRM e smoke sem `pageerror`. A API local tem
seed fixo e bloqueia por padrão toda chamada não interceptada. A classificação
e os limites atuais das suítes estão em
[toka/e2e/README.md](../../toka/e2e/README.md).

## 4. Importante: Artefatos de Teste
Artefatos como relatórios e caches (`playwright-report/`, `test-results/`, `coverage/`, etc.) **NÃO devem ser comitados**. Eles já estão adicionados aos arquivos `.gitignore` para garantir a limpeza do repositório.

## 5. Gates versionados e estado operacional do CI

- PR: os workflows definem testes/lint/coverage gate dos 14 módulos Go, testes/lint/build/budget frontend, checker de RLS, validação documental/Prometheus, smoke da stack e Playwright determinístico.
- Push em `main`: o workflow versionado repete testes/lint/build e publica imagens por SHA, mas não altera a VPS.
- Cobertura é publicada como artefato e cada módulo precisa manter seu próprio baseline; médias globais não escondem regressões.
- Traces e screenshots Playwright são publicados somente quando o gate falha.

Em 18/07/2026, as execuções recentes do GitHub Actions terminam em
`startup_failure` antes de iniciar os jobs; a causa ainda não foi confirmada.
Portanto, a presença do workflow não é evidência de execução verde. Antes de
mergear, execute localmente os gates aplicáveis e registre o resultado.
Produção usa o pipeline direto Hostinger por snapshot Git e SSH, independente
do Actions; o gate de migrations faz parte desse pipeline e exige aprovação
vinculada à SHA quando houver SQL pendente.

As oito antigas specs UI instáveis tiveram o bootstrap estabilizado e agora
fazem parte do gate obrigatório. Apenas as cinco specs full-stack permanecem
explicitamente fora até existir uma composição Supabase efêmera completa; elas
não são apresentadas como testes isolados aprovados.

## 6. Validação completa recomendada

```bash
make check-rls
make test-coverage-gate
make frontend-check
make test-deployment
make test-e2e
```

O último comando exige um navegador instalado pelo Playwright. Ele valida o
gate determinístico; as cinco specs full-stack ainda
exigem ambiente efêmero, Docker e configuração próprios.
