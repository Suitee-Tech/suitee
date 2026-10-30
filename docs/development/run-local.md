# Rodar localmente (E2E)

## 1) Subir infraestrutura + microsserviços (Docker)

```bash
cp .env.example .env
make doctor ENV=local
make up ENV=local
```

Verifique o gateway:

```bash
curl http://localhost:8080/health
```

## 2) Vitrine das imobiliárias (`/site` + site público)

A stack local já inclui `toca-sites`, `toca-web` e o one-shot de migrations do
Payload. Portas: Toka em `3000`, **Payload/Next em `3002`**, `toca-sites` em
`8094`. A marca usada nos exemplos é `Exemplo`, então o host público local é
`exemplo.imoveis.localhost:3002`.

```bash
cp .env.example .env          # se ainda não existir (WEB_PORT=3002)
make sites-check              # valida .env, Docker, portas e resolução do host
make sites-up                 # stack completa: Toka + gateway + toca-sites + toca-web
make sites-smoke              # /health do Go/Next + host público + Payload Admin
make sites-url                # imprime as URLs
```

Para iterar só na vitrine (sem painel Toka/gateway/workers):

```bash
make sites-public-up
```

O `make up` tradicional também sobe os sites; `sites-up` é o atalho com
preflight e as URLs no final. O host `{marca}.imoveis.localhost` resolve
automaticamente no macOS. Em Linux, adicione uma linha em `/etc/hosts`:

```bash
make sites-hosts SITES_SLUG=Exemplo
# ou, para aplicar com sudo:
SITES_HOSTS_APPLY=1 make sites-hosts SITES_SLUG=Exemplo
```

Primeiro acesso:

1. `http://localhost:3002/admin` — crie o primeiro super-admin do Payload
   (o banco está vazio; o primeiro usuário é permitido, os seguintes não).
2. `http://localhost:3000/site` — login no Toka, ligue o site, escolha a marca
   `Exemplo` e salve. O Go provisiona o tenant no Payload e o backfill projeta o
   estoque existente.
3. Em **Contato no site**, confira os dados puxados de Configurações da empresa
   (e-mail, telefone, WhatsApp, endereço e CRECI), ajuste se necessário e salve.
   Eles aparecem no rodapé, no card da imobiliária e nos CTAs de WhatsApp.
4. Publique um imóvel em `/imoveis`; o evento chega pelo RabbitMQ e a vitrine
   aparece em `http://exemplo.imoveis.localhost:3002/imoveis`.
5. O link **Área do cliente** do site aponta para `PORTAL_PUBLIC_URL`
   (`http://localhost:3000/portal` no local). No portal, o deal mostra o cartão
   **Site da imobiliária** com o link de volta para a vitrine.

**Convites e OTP no local.** O `toca-comm` usa o SMTP do Mailpit do Supabase
(`host.docker.internal:54325`) em vez do Resend. Abra `make sites-mail`
(http://localhost:54324), convide o lead em **Leads → Convidar para Portal** e
use o código de acesso que chega na caixa. O convite também preenche o e-mail em
`/portal/login`. Sem esse SMTP local, o Resend rejeita domínios não verificados
e o cliente não recebe nem o convite nem o código.

Se o Mailpit estiver sem a porta SMTP `54325` (volume/config antigo), reinicie o
Supabase uma vez para aplicar `smtp_port = 54325`:

```bash
supabase stop && supabase start
make sites-up
```

Migrações do Payload rodam automaticamente no `up` (serviço
`toca-web-migrate`, dependência de `toca-web`). Para rodar manualmente:

```bash
make sites-migrate
```

Logs e limpeza:

```bash
make sites-status
make sites-logs
make sites-down                 # sem remover volumes
make sites-reset CONFIRM=reset  # remove containers e volumes do Compose (Supabase local permanece)
```

## 3) Frontend

### Opção A — Local (Vite)

```bash
make frontend-install
cd toka && pnpm dev
```

Abra: http://localhost:5173

### Opção B — Docker (mesma origem com proxy)

```bash
make up ENV=local
```

Esse comando, já usado no passo 1, inclui o frontend. Abra:
http://localhost:3000.

## 4) Smoke test rápido (API)

```bash
chmod +x scripts/smoke-test.sh
./scripts/smoke-test.sh
```

## 5) Testes E2E (Playwright)

O gate determinístico inicia o próprio frontend de teste e não depende de
serviços hospedados:

```bash
make test-e2e
```

Para validar a inicialização integrada dos containers com Postgres local:

```bash
make test-stack
```

Consulte a interface completa de build, testes e
deploy para beta, produção, logs por serviço e limpeza
segura.
