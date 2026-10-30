---
title: Roleta de Leads
summary: Distribuição automática de leads por pools e SLA.
tags: [roleta, distribuição, pool, sla, prioridade]
roles: [admin, gestor, operacoes]
---

# Roleta de Leads

## O que é

A roleta distribui leads automaticamente entre os corretores, seguindo
pools, especialidades e regras de SLA. O objetivo é garantir que todo lead
caia na mão certa — e seja atendido rápido, sem depender de divisão manual.

## Onde fica

Menu **CRM & Relacionamento → Roleta de Leads**, rota `/roleta`.
No topo, o botão **Ativar Distribuição** liga a roleta; o badge ao lado do
título mostra o estado (**Roleta Ativa** ou **Roleta Pausada**).

## Passo a passo: configurar pools e SLA

1. Em **Roleta de Leads**, abra a aba **Configuração**.
2. Em **Regras & SLA**, escolha o método de distribuição: **Distribuição
   Sequencial (Roleta)**, **Balanceamento de Carga**, **Peso Ponderado** ou
   **Prioridade por Especialidade**.
3. Ative os critérios que a distribuição deve respeitar: especialidade do
   corretor, região de atuação, horário de expediente e limite de carga
   diária.
4. Em **Níveis de SLA**, ajuste cada nível: **Tempo Limite (min)**,
   **Tentativas Máx** e as **Ações Pós-SLA Expirado** (redirecionar para o
   próximo da fila, pausar o corretor, notificar o gerente, escalar para o
   supervisor ou marcar o lead como urgente).
5. Use **Critérios de Aplicação** (tags) para definir quando cada nível
   vale — ex.: leads "VIP" ou de "Locação".
6. Em **Pools por etapa**, monte os pools indicando quais corretores
   atendem cada etapa da esteira.
7. Em **Qualificação (Forms)**, defina os campos do formulário que a roleta
   usa para casar o lead com o pool certo.
8. Volte ao topo e clique em **Ativar Distribuição**.

*Print: configuração da roleta com pools e SLA.*

## Como o corretor recebe o lead

1. O lead que chega (site, portal ou captação) entra na fila da roleta.
2. A roleta casa os critérios do lead com o pool e atribui ao próximo
   corretor da fila (o **Próximo** aparece destacado no **Monitor Ao Vivo**).
3. O corretor recebe notificação "Novo lead atribuído"; o lead fica em
   **Pendente Aceite** com cronômetro de SLA (padrão 5 minutos).
4. Ao aceitar, o SLA de primeiro contato (padrão 1h) começa a contar e o
   lead muda para **Contatado**.
5. Se o corretor não aceita a tempo, o lead volta para a roleta e é
   redirecionado — o KPI **Redirecionados (Hoje)** registra o evento.

## Pausar e reativar corretores

- Em **Corretores & Fila**, use o botão **Pausar** na linha do corretor
  para tirá-lo da distribuição; use **Reativar** para devolvê-lo ao pool.
- Acompanhe **Corretores Online**, **Leads Hoje**, **Tempo Médio (SLA)** e
  a **Carga de Atendimento** de cada corretor.
- Com a regra de pausa por SLA ativa, quem estoura o prazo fica
  **Pausado (SLA)** automaticamente e os próximos leads pulam para outro
  corretor.
- O **Histórico de Envios** registra cada atribuição, aceite e
  redirecionamento.

## Exemplo prático

> Bruno configura a roleta com pools por especialidade: Carla no pool de
> Locação, Cláudio no de Compra/Venda. Igor preenche o formulário do site
> (Locação, Vila Olímpia, até R$ 4.000) e a roleta atribui o lead a Carla
> em segundos. Carla aceita em 3 minutos, faz a primeira ligação dentro da
> hora e registra a atividade no CRM — o lead segue qualificado, sem
> intervenção manual de ninguém.

## Dicas

- Comece com um pool único e vá criando pools por especialidade/região
  conforme a equipe cresce.
- Revise o **Tempo Médio (SLA)** semanalmente: acima de 30 minutos, ajuste
  a carga ou o tempo limite.
- Não deixe a roleta ativa com todos os corretores pausados — o alerta de
  lead não atribuído avisa o gestor quando isso acontece.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `gestor`, `operacoes` | Configurar pools, SLA, tiers e ativar/pausar a distribuição. |
| `corretor` | Não configura; recebe leads e acompanha a própria carga. |
| `auditor` | Somente leitura da configuração e do histórico. |

## Veja também

- [CRM de Leads](./03-leads.md)
- [Esteira Digital](./06-esteira.md)
- [Agenda](./09-agenda.md)
