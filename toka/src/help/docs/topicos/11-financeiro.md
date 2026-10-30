---
title: Financeiro
summary: Lançamentos, cobranças, recebimentos, comissões, repasses e conferência.
tags: [financeiro, cobrança, recebimento, comissão, repasse, conciliação, lançamento]
roles: [admin, gestor, operacoes, financeiro]
---

# Financeiro

## O que é

O Financeiro concentra o ciclo de valor da imobiliária: lançamentos de receitas
e obrigações, cobranças e recebimentos, comissões de corretores, repasses a
proprietários, contas e encargos, conferência e ordens de pagamento. É aqui que
o mês fecha.

## Onde fica

Menu **Financeiro**, com um item por etapa:

| Item do menu | Rota | O que faz |
|---|---|---|
| Painel financeiro | `/financeiro` | Visão geral: cobranças, repasses e conciliação. |
| Operações | `/financeiro/lancamentos` | Lançamentos e geração por contrato. |
| Recebimentos | `/financeiro/cobrancas` | Cobranças a receber e pagamentos recebidos. |
| Comissões | `/financeiro/comissoes` | Regras, geração e baixa de comissões. |
| Impostos retidos | `/financeiro/impostos` | IRRF/INSS/ISS retidos na comissão: a recolher, vencidos e baixa da guia. |
| Repasses | `/financeiro/repasses` | Repasses a proprietários e parceiros. |
| Contas & encargos | `/financeiro/contas` | Contas de consumo e encargos por imóvel. |
| Conferência | `/financeiro/conciliacao` | Baixas e verificação de integridade. |
| Ordens de pagamento | `/financeiro/payouts` | Aprovação e liquidação de pagamentos. |

## O painel financeiro

O **Painel financeiro** (`/financeiro`) é a visão geral do período: cobranças a
receber, repasses, conciliação e atalhos para cada área do módulo.

*Print: painel financeiro com cobranças e repasses.*

## Passo a passo: registrar um pagamento

1. Abra **Financeiro → Recebimentos** (`/financeiro/cobrancas`).
2. Localize a cobrança nas visões **A apurar**, **Pendentes** ou **Em Atraso**.
3. Clique na linha e escolha **Registrar pagamento**.
4. Informe valor recebido, data e meio de pagamento.
5. Confirme a baixa: a cobrança sai do pendente e vira origem de comissões e
   repasses.

Para reenviar um boleto não recebido, use **Reenviar boleto** na linha da
cobrança.

*Print: tela de recebimentos com baixa.*

## Passo a passo: gerar e baixar comissões

1. Em **Comissões** (`/financeiro/comissoes`), confira a regra do contrato:
   percentual e divisão entre imobiliária e corretor.
2. Use **Gerar contas a pagar para corretores e parceiros** com o recebimento
   já baixado.
3. Revise a distribuição e confirme a geração.
4. Ao pagar o corretor, registre a baixa na linha da comissão.

## Passo a passo: conferir o fechamento

1. Abra **Conferência** (`/financeiro/conciliacao`).
2. Veja as movimentações por estado de baixa: **Aguardando baixa**,
   **Vencidas**, **Baixadas** e **Encerradas**, com os totais de créditos e
   débitos.
3. Leia o painel **Integridade técnica**: ele verifica duplicatas, órfãos,
   excesso de alocação e subalocação sem alterar nenhum registro.
4. Trate cada apontamento antes do fechamento; use **Tentar novamente** para
   repetir a verificação.

## Passo a passo: aprovar uma ordem de pagamento

1. Abra **Ordens de pagamento** (`/financeiro/payouts`).
2. Confira beneficiário, valor e referência de cada ordem pendente.
3. Clique em **Aprovar** nas ordens corretas.
4. Efetue a transferência (PIX/TED) fora da plataforma e anexe o comprovante.

## Exemplo prático

> Eliane fechou a venda do apartamento de Fábio por R$ 800 mil, com comissão de
> 6% (R$ 48 mil: 40% para a imobiliária e 60% para a corretora Carla). Ela
> registra o recebimento em **Recebimentos**, gera a comissão de Carla em
> **Comissões** e cria o repasse a Fábio em **Repasses**, líquido de R$ 752 mil.
> Bruno aprova o repasse e a **Conferência** fecha sem duplicatas nem órfãos.

## Dicas

- Baixe o recebimento antes de gerar comissões e repasses: tudo nasce do pagamento.
- Repasses só seguem após aprovação de um perfil autorizado.
- Rode a Conferência toda semana; não espere o fechamento do mês.
- Guarde o comprovante de cada pagamento feito fora da plataforma.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `financeiro` | Tudo: lançar, baixar, gerar comissões e repasses, aprovar ordens. |
| `operacoes` | Lançar e editar cobranças e comissões; não aprova repasses. |
| `gestor` | Vê todo o módulo, exporta e aprova repasses. |
| `auditor` | Somente leitura. |

## Veja também

- [Inadimplência](./12-inadimplencia.md) — cobranças atrasadas e recuperação.
- [Fiscal e Relatórios](./13-fiscal.md) — NFS-e, DIMOB e exportações.
- [Locação](./08-locacao.md) — origem das cobranças recorrentes.
