---
title: Locação e Manutenção
summary: Contratos de locação, parcelas, encargos, vistorias e manutenção.
tags: [locação, locacao, aluguel, parcela, vistoria, manutenção, fornecedor]
roles: [admin, gestor, corretor, operacoes, financeiro]
---

# Locação e Manutenção

## O que é

O módulo que opera os contratos de aluguel depois da assinatura: parcelas e
cobranças, contas de consumo, lembretes, vistorias de entrada e saída e
ordens de serviço de manutenção com fornecedores.

## Onde fica

Menu **Gestão de Locação**:

- **Locação** → `/locacao`
- **Manutenção** → `/manutencao`

## Passo a passo: operar uma locação ativa

1. Com o contrato **Vigente**, abra a locação em `/locacao/:id` pela aba
   **Visão Geral**.
2. Em **Financeiro**, gere as parcelas: valor do aluguel, quantidade e dia de
   vencimento.
3. Na aba **Contas**, vincule as contas de consumo (luz, água, gás,
   condomínio) e defina quem paga: inquilino ou proprietário.
4. Acompanhe as **Cobranças** e os **Repasses** ao proprietário nas abas
   correspondentes.
5. Configure a **Régua de Lembretes** para avisar o pagador antes e depois do
   vencimento (com envio imediato via **Enviar Lembrete Agora**).

*Print: detalhe da locação com parcelas.*

## Passo a passo: abrir uma vistoria ou ordem de serviço

1. Em **Manutenção**, clique em **Nova Solicitação**.
2. Escolha o tipo: **Manutenção** ou **Vistoria** (de entrada, de saída ou
   periódica).
3. Informe prioridade, categoria, imóvel, contrato e o solicitante.
4. Descreva o problema (ou o objetivo da vistoria) e submeta; a solicitação
   entra com status **Em Análise**.
5. Para vistorias, registre o checklist por cômodo e envie as fotos de cada
   item; o laudo em PDF pode seguir para assinatura na Clicksign.
6. Para manutenções, agende o fornecedor com data e horário e ative a opção
   de notificar o inquilino por WhatsApp.
7. Ao concluir, use o ícone de cobrança para **Lançar no Financeiro** e
   definir quem arca com o custo: inquilino ou proprietário.

*Print: vistoria com itens e fotos.*

## Status e abas

- **Locação**: filtros por status e busca de contratos ativos; o detalhe tem
  as abas **Visão Geral**, **Financeiro**, **Contas**, **Cobranças**,
  **Repasses**, **Vistorias** e **Documentos**.
- **Manutenção**: abas **Todas**, **Manutenção** e **Vistorias**; status
  **Em Análise** → **Em Andamento** → **Concluído** (ou **Cancelado**).

## Exemplo prático

> Gabriela aluga o apartamento da Vila Olímpia por R$ 3.500/mês. Carla faz a
> vistoria de entrada com checklist por cômodo e fotos; o laudo é assinado e
> as chaves entregues. Durante a vigência, Gabriela abre um chamado de
> manutenção por vazamento na cozinha: Diego agenda o encanador Carlos Silva,
> a cobrança de R$ 280 vai para a inquilina e a solicitação é concluída.

## Dicas

- Faça a vistoria de entrada antes de entregar as chaves: ela é a referência
  da vistoria de saída, que compara os itens lado a lado.
- Mantenha as contas de consumo vinculadas: os valores entram direto nas
  deduções do repasse ao proprietário.
- Vistorias periódicas não exigem assinatura — ficam como documento
  informativo do contrato.
- Registre o fornecedor no próprio chamado para manter o histórico completo
  da manutenção.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `gestor`, `operacoes` | Gerir locações, parcelas, contas e encerramentos; abrir e concluir vistorias e ordens de serviço. |
| `corretor` | Ver locações e conduzir vistorias. |
| `financeiro` | Ver e editar locações; processar cobranças, encerramentos e devolução de caução. |
| `auditor` | Somente leitura. |

## Veja também

- [Contratos e Documentos](./07-contratos.md)
- [Agenda](./09-agenda.md)
- [Financeiro](./11-financeiro.md)
- [Portal do Cliente](./18-portal-cliente.md) — parcelas, recibos e chamados pelo portal.
