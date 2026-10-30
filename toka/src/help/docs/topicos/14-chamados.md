---
title: Chamados e Parceiros
summary: Chamados de clientes, triagem, SLA e parceiros.
tags: [chamados, tickets, suporte, sla, parceiros]
roles: [admin, gestor, operacoes]
---

# Chamados e Parceiros

## O que é

Os chamados concentram o suporte ao cliente em uma fila única: o cliente abre
pelo Portal, a operação faz a triagem, responde em tempo real e cumpre o SLA.
Parceiros & Franquias gerencia os acordos com empresas externas que
compartilham portfólio e negócios.

## Onde fica

- **Chamados**: menu **Gestão de Locação → Chamados (Portal)**, rota
  `/chamados`.
- **Parceiros**: menu **Inteligência & Expansão → Parceiros & Franquias**, rota
  `/parceiros`.

*Print: fila de chamados com SLA.*

## Passo a passo: atender um chamado

1. Na fila, abra o chamado novo e leia assunto, prioridade e descrição.
2. Clique em **Atribuir a mim** para assumir a responsabilidade.
3. Clique em **Iniciar** quando começar a tratar (status **Em andamento**).
4. Use **Comentários** para registrar o andamento — o cliente vê na hora, e os
   selos **Operador** e **Cliente** identificam cada autor.
5. Peça informação extra com **Solicitar info** (status **Aguardando cliente**).
6. Ao concluir, clique em **Resolver** e depois **Fechar**.

A fila atualiza em tempo real: um chamado aberto no Portal aparece na sua tela
sem recarregar, com notificação na central de notificações.

## Como o cliente abre um chamado

1. No Portal, o cliente entra em **Chamados** e clica em **Novo chamado**.
2. Escolhe a aba **Suporte** (dúvidas e cobranças) ou **Manutenção** (com fotos).
3. Preenche categoria, prioridade, assunto e descrição e clica em **Abrir
   chamado**.

Chamados de manutenção também aparecem em **Manutenção** (`/manutencao`) para
agendamento de fornecedor e eventual cobrança extra ao inquilino.

## Parceiros & Franquias

Em **Parceiros & Franquias** (`/parceiros`), você convida empresas parceiras ou
franquias, define o que é compartilhado (portfólio, leads) e acompanha os
acordos. Empresas parceiras operam os negócios co-corretados dentro das regras
do acordo.

## Exemplo prático

> Gabriela não recebeu o boleto e abre o chamado "Boleto não chegou" no Portal,
> categoria **Dúvida sobre cobrança**. O chamado aparece na hora na fila de
> Diego, que clica em **Atribuir a mim**, inicia o atendimento e reenvia o
> boleto pelo Financeiro. Ele registra o que fez nos comentários, marca
> **Solicitar info** e Gabriela confirma o pagamento pelo Portal. Diego então
> clica em **Resolver** e **Fechar** — o chamado sai da fila com todo o
> histórico registrado.

## Dicas

- Nunca deixe chamado sem responsável: o SLA corre a partir da abertura.
- Faça a triagem pela prioridade: manutenção urgente e cobrança primeiro.
- Prefira comentários no chamado a conversas soltas: o histórico vira dossiê.
- Atenda os chamados de parceiros pelo mesmo fluxo.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `gestor`, `operacoes` | Atribuir, atender, comentar e fechar chamados. |
| `corretor`, `auditor` | Visualizar a fila de chamados. |
| `admin`, `gestor` | Gerenciar parceiros e franquias. |
| `operacoes`, `auditor` | Visualizar os acordos de parceria. |
| `cliente` | Abrir e acompanhar os próprios chamados pelo Portal. |

## Veja também

- [Portal do Cliente](./18-portal-cliente.md) — onde o cliente abre chamados.
- [Inadimplência](./12-inadimplencia.md) — dúvidas de cobrança viram chamados.
- [Financeiro](./11-financeiro.md) — reenviar boletos e conferir pagamentos.
