---
title: Esteira Digital
summary: Esteira digital de negócios: estágios, aprovações e propostas.
tags: [esteira, deals, negócio, proposta, estágio, aprovação]
roles: [admin, gestor, corretor, operacoes, financeiro]
---

# Esteira Digital

## O que é

A esteira digital organiza os negócios da imobiliária em um kanban por
estágios: **Em Negociação**, **Proposta**, **Análise**, **Contrato** e
**Concluído** (além de **Perdido**). Cada card é um negócio com cliente,
imóvel, valor, propostas, documentos e aprovações.

## Onde fica

Menu **Portfólio & Negócios → Esteira Digital**, rota `/esteira`. O detalhe
de cada negócio fica em `/esteira/deals/:id`.

## Passo a passo: conduzir um negócio na esteira

1. Em **Esteira Digital**, clique em **Nova Proposta** para criar um
   negócio vinculando lead e imóvel (ou converta um lead pelo CRM).
2. Use a busca, os filtros e as visões kanban/lista para localizar cards.
3. Arraste o card entre colunas para mudar o estágio — cada coluna tem
   **Ações do Estágio** no detalhe do card.
4. Abra o card para trabalhar as abas: **Propostas**, **Partes**,
   **Documentos**, **Análise**, **Assinatura** e **Financeiro**.
5. Em **Propostas**, envie a proposta digital; o cliente recebe no Portal
   e a resposta (aceita, recusada ou contraproposta) volta para a esteira.
6. Em **Documentos**, solicite os itens do checklist; cada envio e
   aprovação fica registrado.
7. Em **Análise**, use **Aprovar Análise** (exige permissão de aprovação)
   para liberar a passagem ao **Contrato**; **Reprovar Análise (Perdido)**
   encerra o negócio com motivo.
8. Em **Assinatura**, acompanhe os signatários e o status do envelope;
   com tudo assinado, o negócio avança para **Concluído**.

*Print: kanban da esteira digital.*

## Colaboração em tempo real

- As mudanças são sincronizadas por WebSocket: propostas respondidas no
  Portal, documentos do checklist e assinaturas atualizam o kanban na hora,
  para todos os colaboradores.
- O **Histórico de Alterações** do card mostra quem fez o quê e quando.
- Nos cards com ação pendente, o estágio aparece destacado — não deixe
  negócio parado sem dono da próxima ação.

## Exemplo prático

> Carla converte o lead Gabriela (locação) em negócio na esteira: card em
> **Em Negociação** com o apartamento da Vila Olímpia por R$ 3.500/mês.
> Ela envia a proposta, Gabriela aceita pelo Portal e o card avança para
> **Análise**. Diego solicita RG e comprovantes na aba **Documentos**;
> Gabriela envia, Diego aprova e Bruno (com permissão de aprovação) move o
> card para **Contrato**. Com o contrato assinado, o negócio chega a
> **Concluído** — e a primeira cobrança da locação é gerada no Financeiro.

## Dicas

- Arraste para **Perdido** registrando o motivo: o histórico vira insumo
  para as próximas negociações.
- Vendas acima do limite da empresa exigem aprovação adicional — antecipe
  o gestor antes de prometer prazo ao cliente.
- Use a aba **Financeiro** para conferir a prévia de valores antes de
  concluir o negócio.
- O card com cônjuge faltando trava a transição para **Contrato**:
  cadastre as partes na aba **Partes** antes de avançar.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `gestor`, `corretor`, `operacoes` | Criar, editar e mover negócios entre estágios. |
| `admin`, `gestor`, `operacoes` | Aprovar análises e excluir negócios. |
| `financeiro` | Acompanhar a esteira e aprovar, sem editar. |
| `auditor` | Somente leitura do kanban e do histórico. |

## Veja também

- [CRM de Leads](./03-leads.md)
- [Imóveis e Captações](./05-imoveis.md)
- [Contratos](./07-contratos.md)
- [Financeiro](./11-financeiro.md)
