# Mapa de Integração por Eventos

> Quem publica, quem consome, e que efeito cada evento dispara entre os módulos do monorepo Suitee. Atualizado nas fases 1-3 do plano de integração; manter sincronizado com `pkg/events/contracts/` e os subscribers em `cmd/server/main.go` de cada módulo.

## Convenções

- **Topic**: nome canônico do evento (string), tal qual aparece no `Name()` do contrato.
- **Contrato**: arquivo em `pkg/events/contracts/` (importável por qualquer módulo, sem acoplar serviços).
- **Publicador**: módulo que emite o evento via `eventBus.Publish`.
- **Consumidores**: módulos que assinam via `SubscribeWithQueue` (queue durável) — cada queue recebe a mensagem 1×.
- **Efeito**: o que o consumidor faz ao receber.
- **Orquestrador**: por padrão, `toca-deal` atua como orquestrador (Saga) dos fluxos de contratos/propostas, disparando eventos de downstream.

Eventos não listados aqui ainda existem **localmente** no módulo emissor (não foram movidos para `pkg/events/contracts/`); ver issue de tracking quando sair desse estado.

---

## Eventos shared (em `pkg/events/contracts/`)

### `lead.converted`
- **Contrato**: `lead_events.go` → `LeadConvertedEvent`
- **Publicador**: `toca-crm` quando `lead.is_client = true` é setado.
- **Consumidores**:
  - **`toca-comm`** → notifica account criado no portal.
  - **`toca-deal`** → permite associar deals existentes ao novo cliente via `account_id`.

### `deal.created`
- **Contrato**: `deal_events.go` → `DealCreatedEvent` (parcial — ainda emitido localmente em alguns paths).
- **Publicador**: `toca-deal` em `DealService.Create`.
- **Consumidores**:
  - **`toca-comm`** → ensure channel `deal:{id}` no chat e adiciona buyer + broker como participantes.

### `deal.status_changed`
- **Contrato**: `deal_events.go`
- **Publicador**: `toca-deal` em `DealService.UpdateStatus`.
- **Consumidores**:
  - **`toca-comm`** → in-app notification para o cliente; broadcast WS para refresh.
  - **`toca-core`** → `DealEventHandler` atualiza `Property.ActiveDealID` quando deal converte/cancela.

### `deal.contract.signed`
- **Contrato**: ainda local em `toca-deal`.
- **Publicador**: `toca-deal` quando todos os signatários assinam.
- **Consumidores**:
  - **`toca-comm`** → notificação "Contrato Assinado" para todas as partes.

### `contract.signed`
- **Contrato**: `deal_events.go` → `ContractSignedEventPayload`
- **Publicador**: `toca-deal` após contrato definitivo ser registrado.
- **Consumidores**:
  - **`toca-financial`** → orquestra:
    1. Charge inicial (sale) ou config de cobrança recorrente (rent).
    2. Owner Transfer.
    3. Broker Commissions (split por participation).
    4. **Phase 2.6**: cria `Rental` agregado + emite `rental.created`.

### `proposal.sent`
- **Contrato**: `proposal_events.go`
- **Publicador**: `toca-deal` quando proposta é enviada ao cliente.
- **Consumidores**:
  - **`toca-comm`** → in-app + WS para o cliente.

### `proposal.responded`
- **Contrato**: `proposal_events.go`
- **Publicador**: `toca-deal` quando cliente aceita/rejeita.
- **Consumidores**:
  - **`toca-comm`** → in-app para operadores no canal do deal.

### `signer.invited` / `signer.signed`
- **Contrato**: `signer_events.go`
- **Publicador**: `toca-deal`.
- **Consumidores**: `toca-comm` (notifica próxima parte na fila de assinaturas).

### `lead.created` / `lead.assigned` / `lead.escalated` / `lead.status_changed`
- **Contrato**: parcial em `lead_events.go`; restante ainda local em `toca-crm`.
- **Publicador**: `toca-crm`.
- **Consumidores**:
  - **`toca-comm`** → notifica broker designado, gestor (se SLA escalou), broadcast WS pra dashboard live.

### `client.ticket.created` / `ticket.created` / `ticket.status_changed` / `ticket.comment_added`
- **Contrato**: `ticket_events.go`
- **Publicador**: `toca-deal` (chamados do portal cliente).
- **Consumidores**:
  - **`toca-comm`** → notifica operadores no canal correspondente.

### `client.appointment.requested` / `appointment.updated` / `appointment.cancelled`
- **Contrato**: `calendar_events.go`
- **Publicador**: `toca-deal` (portal cliente) e `toca-calendar`.
- **Consumidores**:
  - **`toca-comm`** → in-app para a outra parte.

### `invoice.paid`
- **Contrato**: `invoice_events.go`
- **Publicador**: `toca-financial`.
- **Semântica**: liquidação de uma `Invoice` real; carrega `invoice_id`,
  `charge_id` e o valor daquela invoice, mesmo quando a alocação é parcial em
  relação à cobrança canônica.
- **Consumidores**:
  - **`toca-comm`** → notificação "Pagamento confirmado" para o cliente.
- **Compatibilidade**: `schema_version=2` identifica o contrato atual. O
  consumidor financeiro mantém temporariamente uma fila legada apenas para
  mensagens sem versão antes emitidas pelo `ChargeService`; eventos v2 não
  liberam payout de cobrança.

### `charge.paid`
- **Contrato**: `financial_events.go` → `ChargePaidEvent`.
- **Publicador**: `toca-financial` (`ChargeService`), somente quando o saldo da
  cobrança canônica chega a zero.
- **Semântica**: quitação integral da `Charge`; não contém `invoice_id`, pois a
  origem pode ser uma ou mais invoices ou conciliação manual. (Webhook PSP é
  fora de escopo — decisão FIN-E2E-35; o fluxo de pagamento é manual.)
- **Consumidores**:
  - **`toca-financial`** → orquestra os efeitos idempotentes dependentes da
    quitação integral.

### `checklist.solicited`
- **Contrato**: ainda local.
- **Publicador**: `toca-deal` quando broker solicita um item do checklist.
- **Consumidores**:
  - **`toca-comm`** → in-app para a parte específica.

---

## Eventos novos da Fase 3

### `rental.created`
- **Contrato**: `rental_events.go` → `RentalCreatedEvent`
- **Publicador**: `toca-financial` em `RentalService.CreateFromDeal` (disparado pelo handler de `contract.signed` quando `dealType=RENT`).
- **Consumidores**:
  - **`toca-comm`** → in-app "Locação iniciada" para tenant_lead, "Imóvel locado" para owner_lead.
- **Idempotência**: garantida por `(tenant_id, deal_id)` em `RentalRepository.GetByDealID` antes de criar.

### `rental.charge_overdue`
- **Contrato**: `rental_events.go` → `RentalChargeOverdueEvent`
- **Publicador**: `toca-financial` (`DelinquencyService` ou job batch).
- **Consumidores**:
  - **`toca-comm`** → in-app urgente para tenant_lead + nudge para owner_lead.

### `maintenance.requested`
- **Contrato**: `maintenance_events.go` → `MaintenanceRequestedEvent`
- **Publicador**: `toca-ops` quando ticket de manutenção é criado.
- **Consumidores**:
  - **`toca-financial`** → cria Charge automática no payer (tenant ou owner conforme `payer_role`).
  - **`toca-comm`** → notifica broker designado e parte oposta no canal.

### `maintenance.completed`
- **Contrato**: `maintenance_events.go` → `MaintenanceCompletedEvent`
- **Publicador**: `toca-ops` quando ticket vai para `Concluído`.
- **Consumidores**:
  - **`toca-financial`** → reconcilia Charge com `actual_cost` se diverge de `estimated_cost`.
  - **`toca-comm`** → in-app "Manutenção concluída" para requester_lead.

### `property.captured`
- **Contrato**: `captacao_events.go` → `PropertyCapturedEvent`
- **Publicador**: `toca-core` no endpoint `POST /captacao`.
- **Consumidores**:
  - **`toca-comm`** → in-app "Captação iniciada" para owner_lead.
- **Quando dispara**: na criação de Property com `status=prospecting` + `capturing_brokers[]`.

### `property.capturing_broker_added`
- **Contrato**: `captacao_events.go` → `CapturingBrokerAddedEvent`
- **Publicador**: `toca-core` em `POST /properties/:id/capturing-brokers`.
- **Consumidores**: ainda nenhum (reservado para futuras integrações de comissão).

---

## Fluxo end-to-end de uma locação RENT

```
toca-crm           lead.created (owner)              → comm: notifica equipe
toca-core          property.captured                 → comm: notifica owner
toca-crm           lead.created (tenant)             → comm: notifica equipe
toca-deal          deal.created (RENT)               → comm: cria channel
toca-deal          deal.status_changed (CONTRACT)    → comm + core (active_deal)
toca-deal          contract.signed                   → financial:
                                                          ├─ rental_charge_config + charges
                                                          ├─ owner_transfer
                                                          ├─ broker_commissions
                                                          └─ rental aggregate    (Phase 2.6)
toca-financial     rental.created                    → comm: notifica tenant + owner
toca-ops           maintenance.requested             → financial: charge auto (se payer_role)
                                                     → comm: notifica broker
toca-financial     rental.charge_overdue (cron)      → comm: alerta tenant + owner
```

---

## Como adicionar um novo evento

1. Criar struct `<Name>EventPayload` + `<Name>Event` em `pkg/events/contracts/<area>_events.go`.
2. Implementar `EventID()`, `Name()`, `Payload()` (ver padrão em `lead_events.go`).
3. Publicar via `eventBus.Publish(ctx, evt)` no módulo emissor.
4. Subscribir via `eventBus.SubscribeWithQueue("<consumer>.<event_name>", "<event_name>", handler)` no consumer (queue prefixada por módulo permite redeliver isolado por consumer).
5. **Atualizar este arquivo** — coluna Topic, Publicador, Consumidores e Efeito.

## Observabilidade

- Outbox + DLQ implementados em `pkg/events/rabbitmq.go`. Mensagens que falham N vezes vão pra `<queue>.dlq` e podem ser reexecutadas com `rb.StartDLQConsumers()`.
- Cada evento carrega `EventID` (usado como `message_id` no AMQP) — permite dedup no consumidor.
- Logs estruturados em todos os subscribers via `pkg/logger`.
