# Inventário físico

Estado reconstruído do histórico versionado em 25/09/2026. Partições filhas de `audit.events` não são contadas como entidades adicionais.

| Schema | Tabelas | Colunas | Responsabilidade |
| --- | ---: | ---: | --- |
| `ai` | 3 | 30 | Conversas, mensagens e chamadas de ferramentas de IA |
| `audit` | 1 | 11 | Eventos de auditoria, particionados por data |
| `calendar` | 1 | 6 | Agendamentos e visitas |
| `comm` | 5 | 29 | Canais, mensagens, notificações e deduplicação de envio |
| `core` | 8 | 50 | Imóveis, parcerias e configurações da empresa |
| `crm` | 6 | 37 | Leads, corretores, roleta e quarentena de telefones |
| `deal` | 4 | 25 | Negociações, documentos, modelos e contratos |
| `events` | 3 | 24 | Outbox, deduplicação por consumidor e fila de falhas |
| `exports` | 1 | 10 | Jobs de exportação |
| `financial` | 27 | 206 | Cobranças, locações, comissões, repasses, razão e registros fiscais |
| `ops` | 6 | 36 | Manutenção, chaves, vistorias, chamados, fornecedores e ledger de deploy |
| `portal` | 4 | 24 | Integrações externas, histórico de sincronização, leads e cache de feeds |
| `signature` | 2 | 25 | Envelopes e eventos recebidos do provedor de assinatura |
| `toca_auth` | 7 | 34 | Contas, credenciais, convites, grupos e preferências |

## Tabelas por schema

### ai

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `conversations` | 8 | `id` | Sim |
| `messages` | 11 | `id` | Sim |
| `tool_invocations` | 11 | `id` | Sim |

### audit

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `events` | 11 | `id, created_at` | Sim |

### calendar

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `appointments` | 6 | `id` | Sim |

### comm

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `channels` | 6 | `id` | Sim |
| `inapp_notifications` | 6 | `id` | Sim |
| `messages` | 6 | `id` | Sim |
| `sent_externals` | 5 | `event_id, channel, recipient_id` | Sim |
| `templates` | 6 | `id` | Sim |

### core

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `api_keys` | 6 | `id` | Sim |
| `appearance_settings` | 5 | `tenant_id` | Sim |
| `company_settings` | 5 | `tenant_id` | Sim |
| `notification_settings` | 5 | `tenant_id` | Sim |
| `partnerships` | 8 | `id` | Sim |
| `properties` | 10 | `id` | Sim |
| `role_permissions` | 5 | `tenant_id` | Sim |
| `webhooks` | 6 | `id` | Sim |

### crm

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `brokers` | 6 | `id` | Sim |
| `lead_phone_quarantine` | 6 | `id` | Sim |
| `leads` | 7 | `id` | Sim |
| `roulette_audit` | 8 | `id` | Sim |
| `roulette_configs` | 4 | `tenant_id` | Sim |
| `roulette_history` | 6 | `id` | Sim |

### deal

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `contracts` | 7 | `id` | Sim |
| `deals` | 6 | `id` | Sim |
| `documents` | 6 | `id` | Sim |
| `templates` | 6 | `id` | Sim |

### events

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `dlq_events` | 11 | `message_id, dlq_name` | Sim |
| `outbox_events` | 10 | `id` | Sim |
| `processed_events` | 3 | `consumer, message_id` | Sim |

### exports

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `jobs` | 10 | `id` | Sim |

### financial

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `charges` | 6 | `id` | Sim |
| `collections` | 6 | `id` | Sim |
| `commissions` | 6 | `id` | Sim |
| `dimob` | 6 | `id` | Sim |
| `fiscal_deadlines` | 6 | `id` | Sim |
| `informes` | 6 | `id` | Sim |
| `invoices` | 6 | `id` | Sim |
| `journal_entries` | 6 | `id` | Sim |
| `managed_bills` | 37 | `id` | Sim |
| `nfse` | 6 | `id` | Sim |
| `partner_royalties` | 15 | `id` | Sim |
| `payments` | 6 | `id` | Sim |
| `payout_orders` | 6 | `id` | Sim |
| `platform_subscriptions` | 6 | `id` | Sim |
| `reminder_rules` | 6 | `id` | Sim |
| `rental_charge_configs` | 6 | `id` | Sim |
| `rentals` | 6 | `id` | Sim |
| `report_history` | 6 | `id` | Sim |
| `sale_billing_plan` | 6 | `id` | Sim |
| `sale_billing_plan_drafts` | 7 | `id` | Sim |
| `tenant_commission_rules` | 7 | `id` | Sim |
| `tenant_financial_settings` | 6 | `id` | Sim |
| `transactions` | 6 | `id` | Sim |
| `transfer_deduction_claims` | 8 | `tenant_id, reference_id` | Sim |
| `transfers` | 6 | `id` | Sim |
| `utility_accounts` | 6 | `id` | Sim |
| `withdrawals` | 6 | `id` | Sim |

### ops

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `deployment_migration_checksums` | 6 | `version` | Sim |
| `inspections` | 6 | `id` | Sim |
| `keys` | 6 | `id` | Sim |
| `maintenance` | 6 | `id` | Sim |
| `suppliers` | 6 | `id` | Sim |
| `tickets` | 6 | `id` | Sim |

### portal

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `feed_cache` | 6 | `id` | Sim |
| `integrations` | 6 | `id` | Sim |
| `leads` | 6 | `id` | Sim |
| `sync_history` | 6 | `id` | Sim |

### signature

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `envelopes` | 14 | `id` | Sim |
| `webhook_events` | 11 | `id` | Sim |

### toca_auth

| Tabela | Colunas | PK | RLS habilitado |
| --- | ---: | --- | --- |
| `accounts` | 4 | `id` | Sim |
| `credentials` | 6 | `id` | Sim |
| `groups` | 5 | `id` | Sim |
| `invitations` | 9 | `id` | Sim |
| `password_resets` | 3 | `id` | Sim |
| `tenant_settings` | 5 | `tenant_id` | Sim |
| `user_preferences` | 2 | `account_id` | Sim |

