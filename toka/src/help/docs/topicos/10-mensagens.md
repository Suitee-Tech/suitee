---
title: Mensagens e Notificações
summary: Canais de mensagem, Slack e central de notificações.
tags: [mensagens, notificações, slack, canal, chat]
roles: [admin, gestor, corretor, operacoes, marketing]
---

# Mensagens e Notificações

## O que é

A central de comunicação da imobiliária: conversas em canais vinculados ao
negócio (lead, deal, imóvel, contrato, suporte), integração com Slack e a
central de notificações com todos os avisos do sistema.

## Onde fica

- **Mensagens**: menu **CRM & Relacionamento → Mensagens**, rota `/mensagens`.
- **Notificações**: sino no topo da tela, rota `/notificacoes`.

## Passo a passo: conversar em um canal

1. Em **Mensagens**, escolha o canal na lista lateral ou crie um novo.
2. Confira o tipo do canal — **direct** (conversa direta), **group** (grupo),
   **support** (suporte), **lead** ou **deal** — e o vínculo com o cliente,
   negócio, imóvel ou contrato.
3. Digite a mensagem e envie; o destinatário recebe em tempo real.
4. Acompanhe o status da mensagem: **Enviando...**, **Entregue** ou
   **Falhou** — neste caso, use **Reenviar**.
5. Anexe arquivos quando precisar; eles ficam guardados no canal.

*Print: canal de mensagens vinculado a um lead.*

## Integração com Slack

- Vincule o Slack da equipe em **Configurações → Integrações** (webhooks).
- Com a ponte ativa, canais externos conversam com a central de mensagens.
- A configuração é feita por `admin`, `operacoes` ou `marketing`.

## Central de notificações

1. Em **Notificações**, veja os avisos de novos leads, chamados, comentários,
   mensagens e eventos de negócio.
2. Filtre por **Todas** ou **Não lidas** e use **Marcar ... como lidas**.
3. Clique na notificação para abrir direto o que gerou o aviso (ex.: o
   chamado em `/chamados`).

## Exemplo prático

> Gabriela abre um chamado no portal ("Boleto não chegou"). Diego recebe a
> notificação em **Notificações**, atribui o chamado a si e responde no canal
> **Suporte - Apto Vila Olímpia**, vinculado ao contrato: "Olá Gabriela, vou
> verificar o envio do boleto." A mensagem chega em tempo real no portal da
> cliente, ela paga e confirma, e Diego resolve e fecha o chamado — tudo
> registrado na trilha de auditoria.

## Dicas

- Prefira canais vinculados (lead/deal) a conversas soltas: o histórico fica
  no contexto do cliente.
- Se a mensagem falhar por instabilidade de rede, clique em **Reenviar** em
  vez de digitar tudo de novo.
- Marque as notificações como lidas para zerar o sino do topo.
- Cliente sem notificações push ativas recebe a mensagem ao abrir o portal;
  o e-mail funciona como reforço.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `gestor`, `corretor`, `operacoes`, `marketing` | Ler e enviar mensagens nos canais do módulo CRM. |
| `admin`, `operacoes`, `marketing` | Configurar a integração com Slack. |
| `auditor` | Somente leitura nos canais. |

## Veja também

- [CRM de Leads](./03-leads.md)
- [Locação e Manutenção](./08-locacao.md)
- [Portal do Cliente](./18-portal-cliente.md) — mensagens e chamados pelo portal.
