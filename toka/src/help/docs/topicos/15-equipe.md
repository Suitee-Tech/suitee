---
title: Equipe, Permissões e Auditoria
summary: Membros, perfis, permissões e trilha de auditoria.
tags: [equipe, membros, perfis, permissões, auditoria, audit trail]
roles: [admin, gestor, auditor]
---

# Equipe, Permissões e Auditoria

## O que é

A área de administração organiza quem acessa a Suitee: as empresas
(workspaces) da sua conta, os membros da equipe com seus perfis de permissão e
a trilha de auditoria, que registra as ações relevantes feitas na plataforma.

## Onde fica

- **Empresas**: menu **Administração → Empresas**, rota `/empresas`.
- **Trilha de auditoria**: menu **Administração → Audit Trail**, rota
  `/audit-trail`.

## Passo a passo: convidar um membro da equipe

1. Em **Administração → Empresas**, abra a empresa desejada.
2. Clique em **Convidar membro** e informe o e-mail profissional da pessoa.
3. Escolha o perfil funcional (ex.: `corretor`, `gestor`, `operacoes`).
4. Envie o convite. A pessoa recebe o link por e-mail e, ao aceitar, entra no
   workspace com as permissões do perfil escolhido.

## Passo a passo: trocar de empresa

1. Clique no seu avatar no topo do app e escolha **Trocar empresa**.
2. Na tela **Seleção de empresa**, clique no workspace desejado.
3. O app abre a empresa escolhida, com menus e dados próprios — nada de uma
   empresa aparece na outra.

## Perfis funcionais

- `admin` (Administrador) — acesso total; gerencia permissões, integrações e
  assinatura.
- `gestor` (Gestor Comercial) — CRM, esteira, contratos e equipe; aprova
  negócios e repasses.
- `corretor` (Corretor) — CRM, imóveis, leads e negócios; participa da roleta
  e assina contratos.
- `operacoes` (Operações & Compliance) — visão ampla de leads, imóveis e
  contratos; valida documentos.
- `financeiro` (Financeiro) — cobranças, repasses e fiscal; aprova
  transferências.
- `marketing` (Marketing) — leads, imóveis, integrações e relatórios; sem
  escrita financeira.
- `auditor` (Auditor / Leitor) — leitura ampla e trilha de auditoria; sem
  escrita.
- `parceiro` (Parceiro / Stakeholder) — portfólio, imóveis (parcial) e
  contratos como signatário; acessa o Portal.
- `cliente` (Cliente Final) — Portal isolado com imóveis, locações, chamados
  e contratos como signatário.

*Print: gestão de membros e permissões.*

## Passo a passo: acompanhar a trilha de auditoria

1. Abra **Administração → Audit Trail**.
2. Filtre por **Entidade** (ex.: lead, contrato), **Ação** e **Período**.
3. Use a busca para localizar um usuário ou registro específico.
4. Clique na seta de uma entrada para expandir o detalhe: autor, horário e o
   que mudou (antes e depois).
5. Para gerar evidência, clique em **Exportar CSV** (planilha) ou
   **Exportar PDF** (relatório).

## Exemplo prático

> Bruno reclama que a corretora Carla recebeu um lead que deveria ir para
> Cláudio. Norberto, auditor, abre o Audit Trail, filtra por entidade *lead* e
> período de 24 horas e localiza as entradas do lead: criado pelo site às
> 14:23, atribuído pela roleta no mesmo minuto e aceito por Carla às 14:25. Ao
> expandir a atribuição, vê que Carla foi incluída por engano no pool de
> Compra/Venda às 10:00. Norberto exporta o CSV como evidência e Bruno corrige
> a regra — correção que também fica registrada na trilha.

## Dicas

- Revise os perfis da equipe a cada mudança de time: perfil errado é a causa
  mais comum de "falta de acesso".
- Convites são pessoais e valem para um único e-mail.
- A trilha de auditoria é a fonte oficial para investigar incidentes e
  atender pedidos de privacidade.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin` | Convidar e remover membros, definir perfis e gerenciar empresas. |
| `gestor`, `operacoes` | Gerenciar membros da equipe e ler a trilha. |
| `financeiro` | Ler a trilha de auditoria. |
| `auditor` | Somente leitura, incluindo a trilha de auditoria. |
| demais | Sem acesso à área de administração. |

## Veja também

- [Começando na Suitee](./01-comecando.md)
- [Configurações e Assinatura](./16-configuracoes.md)
- [Privacidade e LGPD](./19-seguranca-lgpd.md)
