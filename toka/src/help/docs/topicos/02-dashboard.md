---
title: Painel inicial
summary: O painel inicial: KPIs, agenda do dia e leads recentes.
tags: [painel, dashboard, kpi, agenda, home]
roles: [admin, gestor, corretor, operacoes, financeiro, marketing, auditor]
---

# Painel inicial

## O que é

O painel é a tela inicial da Suitee, montada para o seu dia de trabalho:
resume os KPIs da operação, os compromissos de hoje, os leads que acabaram
de chegar e as negociações em andamento. Cada usuário vê os números do seu
escopo — o corretor enxerga a própria carteira, o gestor enxerga a equipe.

## Onde fica

Item **Painel** no menu lateral, rota `/`. É a primeira tela após o login.

## Passo a passo: acompanhar o dia no painel

1. Abra o **Painel** e confira o cabeçalho: saudação, empresa e data.
2. Leia os quatro KPIs do topo: **Leads Ativos**, **Visitas Hoje**,
   **Propostas Pendentes** e **Comissões (Est.)**.
3. Revise a **Agenda do Dia**: cada compromisso aparece com horário, tipo
   (visita, ligação, reunião) e cliente.
4. Veja **Leads Recentes**: os leads novos e em atendimento, com origem,
   interesse e atalhos de **WhatsApp** e **Ver Ficha**.
5. Acompanhe **Negociações Ativas**: seus negócios em andamento com imóvel,
   valor e estágio atual.
6. Compare as **Metas do Mês**: **VGV Vendido** e **Taxa de Conversão**.

*Print: painel inicial com KPIs e agenda do dia.*

## Atalhos

- **Atender Novo Lead** → abre a lista de leads (`/leads`).
- **Ver todas** e **+ Adicionar Tarefa** (Agenda do Dia) → abrem a agenda.
- **Ver todos os leads ativos** → abre o CRM de leads.
- **Ir para Esteira Digital** → abre a esteira de negócios.
- Clicar em um compromisso abre o detalhe dele na agenda; clicar em um lead
  ou negociação abre a ficha correspondente.

## Exemplo prático

> Bruno, gestor, começa a segunda-feira no painel: vê 42 leads ativos, 3
> visitas hoje e R$ 96.000 em comissões estimadas. Na agenda, nota a visita
> das 10h de Carla com a locatária Gabriela. Em Leads Recentes, um lead novo
> de locação chegou pelo site há 5 minutos — ele clica em **Atender Novo
> Lead**, abre a ficha e encaminha para a roleta antes das 9h30.

## Dicas

- O KPI **Comissões (Est.)** só é carregado para quem tem acesso financeiro;
  sem ele, o card não aparece.
- Use **Visitas Hoje** como termômetro da operação: dia sem visita é sinal
  de funil parado.
- Em dúvida sobre algum número, pergunte à Suitee IA no módulo Inteligência.
- Cobranças e repasses ficam no menu **Financeiro**, não no painel.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `gestor`, `operacoes`, `marketing`, `auditor` | Ver KPIs, agenda, leads recentes e metas do time. |
| `corretor` | Ver o painel com os próprios números (carteira e comissões). |
| `financeiro` | Ver o painel e o KPI de comissões, se tiver acesso financeiro. |
| `auditor` | Acesso em modo somente leitura. |

## Veja também

- [CRM de Leads](./03-leads.md)
- [Esteira Digital](./06-esteira.md)
- [Agenda](./09-agenda.md)
- [Financeiro](./11-financeiro.md)
