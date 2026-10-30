# Suítes E2E

Os testes Playwright estão classificados em `suites.json`. O verificador
`scripts/check-e2e-classification.mjs` falha quando uma spec nova não pertence a exatamente uma suíte.

## Gate determinístico de pull request

`playwright.ci.config.ts` executa 11 specs comprovadamente verdes e isoladas (174 testes): login,
recuperação de senha, cadastro em quatro passos, onboarding, dashboard, CRM de leads e um smoke
explícito do artefato de produção que falha diante de qualquer `pageerror`.

A execução sobe somente dois processos locais:

1. uma build de teste servida pelo Vite Preview em `127.0.0.1:4173`;
2. uma API de contenção em `127.0.0.1:4174`, inicializada com `E2E_SEED=suitee-ci-v1`.

A API local é *deny by default*. `vite.e2e.config.ts` encaminha `/api/v1` para ela, mantendo as
requisições do navegador na mesma origem para que os mocks do Playwright também cubram preflights.
Qualquer chamada que a spec não interceptar recebe `501` e faz o teardown falhar. URLs de mídia e
Supabase também apontam para esse processo local, portanto o gate não usa projeto Supabase, banco ou
serviço SaaS externo. Os dados de cada jornada são fixtures fixas das próprias specs. Traces e
screenshots ficam em `test-results/deterministic-ui` e o workflow só envia esse diretório quando a
execução falha.

A API local responde `POST /api/v1/auth/refresh = 401`, representando o navegador sem sessão, e
leituras `GET /api/v1/*` do shell autenticado com o seed vazio. Qualquer mutação não interceptada
continua falhando, sem acesso a um backend real.

As oito specs que antes estavam em `mockedUiDeferred` foram estabilizadas e promovidas ao gate em
17/07/2026. A causa do bloqueio autenticado era o fixture de preferências sem a versão vigente do
consentimento, que deixava o conteúdo atrás do modal global. A home também usava data UTC para uma
regra baseada no dia local, e o onboarding tentava operar o seletor Radix como um `<select>` nativo.
Os fixtures e as interações acessíveis foram corrigidos sem remover as asserções. O `SelectField`
agora encaminha `onBlur`, portanto a validação obrigatória de UF também é exercitada. A classificação
`mockedUiDeferred` permanece vazia e explícita no manifesto para futuras quarentenas justificadas.

Execução local:

```bash
pnpm install --frozen-lockfile
./node_modules/.bin/playwright install chromium
E2E_SEED=suitee-ci-v1 ./node_modules/.bin/playwright test --config=playwright.ci.config.ts
```

Para validar com um Google Chrome já instalado localmente, sem baixar o Chromium do Playwright:

```bash
E2E_BROWSER_CHANNEL=chrome E2E_SEED=suitee-ci-v1 \
  ./node_modules/.bin/playwright test --config=playwright.ci.config.ts
```

O gate agregado foi revalidado com Google Chrome local: 174/174 testes passaram usando o seed
`suitee-ci-v1`, sem mutações inesperadas na API de contenção.

## Specs full-stack (não executadas no gate de PR)

`auth.spec.ts`, `health.spec.ts`, `deal-pipeline.spec.ts`, `roulette-pipeline.spec.ts` e
`contract-financial.spec.ts` exercitam o gateway real, persistência, RabbitMQ e múltiplos
microsserviços. Elas não são equivalentes às specs de UI mockada.

A composição atual não constitui uma stack Supabase local completa: o Postgres do Compose carrega
somente um subconjunto das migrations e não sobe Auth/GoTrue, Storage e os demais serviços Supabase.
Por isso essas specs não apontam para o projeto Supabase hospedado durante pull requests e não são
rotuladas como “isoladas”. Até existir uma composição completa com todas as migrations e um seed
idempotente, elas devem ser executadas apenas contra um ambiente efêmero controlado:

```bash
E2E_BASE_URL=http://127.0.0.1:5173 \
E2E_API_URL=http://127.0.0.1:8080/api/v1 \
./node_modules/.bin/playwright test \
  e2e/auth.spec.ts \
  e2e/health.spec.ts \
  e2e/deal-pipeline.spec.ts \
  e2e/roulette-pipeline.spec.ts \
  e2e/contract-financial.spec.ts
```

`contract-financial.spec.ts` é deliberadamente mutável e falha fechado: além das URLs, exige
`E2E_MUTABLE_FINANCIAL=1` e `E2E_ISOLATED_DATABASE=1`. A jornada cria um tenant exclusivo, assina o
contrato, gera cobranças, registra dois recebimentos parciais (incluindo retry idempotente), comprova
que não há repasse antes da quitação, verifica o repasse limitado ao caixa, o ledger, o DRE e a
reconciliação. Os recursos upstream removíveis são apagados no `afterAll`; o histórico financeiro,
por ser imutável, é removido descartando o banco efêmero ao final. Nunca habilite essas variáveis
contra beta, produção ou uma stack local compartilhada.

Esse bloqueio full-stack deve ser fechado antes de promover essas cinco specs ao gate obrigatório.
