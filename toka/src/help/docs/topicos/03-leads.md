---
title: CRM de Leads
summary: Captar, qualificar, atribuir e converter leads no CRM.
tags: [leads, captação, qualificação, conversão, busca, importação]
roles: [admin, gestor, corretor, operacoes, marketing]
---

# CRM de Leads

## O que é

O CRM centraliza todos os contatos e oportunidades da imobiliária: leads que
chegam do site, de portais, de indicações ou de captação ativa. Daqui você
qualifica, atribui a corretores e converte em negócio na esteira.

## Onde fica

Menu **CRM & Relacionamento → Leads (CRM)**, rota `/leads`.

## Passo a passo: cadastrar um lead

1. Em **Leads**, clique em **Novo lead**.
2. Escolha o tipo da captação: **Captação** (imóvel para vender/alugar),
   **Compra e venda** ou **Locação**.
3. Preencha os dados de contato (nome, e-mail, telefone) e o interesse
   (bairro, valor, quartos) quando houver.
4. Defina o corretor responsável ou deixe para a roleta distribuir.
5. Salve. O lead entra no pipeline com o status **Novo**.

## Passo a passo: qualificar e converter

1. Abra o detalhe do lead (`/leads/:id`) e registre cada interação na timeline
   (ligação, WhatsApp, visita).
2. Avance o status conforme o funil: Novo → Contatado → Qualificado →
   Visita agendada → Proposta → Negociação.
3. Ao fechar o interesse, use **Converter em negócio** para criar o deal na
   esteira (veja [Esteira Digital](./06-esteira.md)).

## Busca, filtros e visões

- **Busca**: nome, e-mail, telefone ou código do lead.
- **Filtros**: status, origem, corretor, interesse (bairro, faixa de valor,
  quartos) e datas de atribuição.
- **Visões**: lista ou quadro (kanban por status).
- **Importação**: planilha com os campos de contato; o sistema valida
  duplicados e atribui à roleta quando não há corretor definido.

## Exemplo prático

> Bruno, corretor, recebe da roleta o lead Carla — interesse em apartamento de
> 2 quartos até R$ 500 mil na Vila Mariana. Ele registra a primeira ligação na
> timeline, agenda uma visita pela Agenda e marca o status **Visita agendada**.
> Após a visita, avança para **Proposta**, anexa a proposta no detalhe e, com a
> aceitação, converte em negócio na esteira.

## Dicas

- Não deixe lead sem responsável: ative a roleta para distribuir
  automaticamente (veja [Roleta](./04-roleta.md)).
- Use as tags para marcar leads quentes (ex.: "urgente", "indicação").
- Registre toda interação na timeline — o histórico vira o dossiê do cliente.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `gestor`, `corretor`, `operacoes`, `marketing` | Criar, editar e mover leads. |
| `admin`, `gestor`, `operacoes` | Reatribuir e excluir leads. |
| `auditor` | Somente leitura. |

## Veja também

- [Roleta de Leads](./04-roleta.md)
- [Esteira Digital](./06-esteira.md)
- [Agenda](./09-agenda.md)
