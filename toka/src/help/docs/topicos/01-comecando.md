---
title: Começando na Suitee
summary: Login, cadastro, convites, primeiro acesso e seleção de empresa.
tags: [login, senha, cadastro, convite, onboarding, empresa, primeiro acesso]
roles: [admin, gestor, corretor, operacoes, financeiro, marketing, auditor]
---

# Começando na Suitee

## O que é

A Suitee é a plataforma completa da imobiliária: CRM de leads, portfólio de
imóveis, esteira de negócios, contratos com assinatura eletrônica, locação,
financeiro, fiscal e o portal dos clientes. Este tópico cobre o acesso e os
primeiros passos.

## Onde fica

- **Login**: [app.example.com](https://app.example.com/login).
- **Cadastro**: mesma tela, aba **Cadastre-se**.

## Passo a passo: entrar pela primeira vez

1. Acesse a tela de login com o e-mail profissional e a senha cadastrados.
2. Se você foi convidado por e-mail, abra o link do convite (`/invite`) e siga
   o fluxo: quem já tem conta apenas entra; quem não tem cria a conta na hora.
3. No primeiro acesso de uma empresa nova, o **onboarding da imobiliária**
   aparece em 3 passos: dados da empresa, identidade visual e primeiro usuário
   de equipe — preencha para liberar o menu completo.

## Passo a passo: cadastrar-se

1. Na tela de login, selecione a aba **Cadastre-se**.
2. **Passo 1 — Contato**: informe e-mail e telefone; o sistema confirma a
   disponibilidade antes de continuar.
3. **Passo 2 — Identidade**: nome, sobrenome e cargo.
4. **Passo 3 — Senha**: crie a senha e a confirmação.
5. **Passo 4 — Revisão**: aceite os termos e conclua o cadastro.

## Passo a passo: trocar de empresa (multi-tenant)

1. Se a sua conta pertence a mais de uma imobiliária, a tela **Seleção de
   empresa** (`/company-selection`) aparece ao entrar.
2. Escolha o workspace desejado ou clique em **Cadastrar nova empresa** para
   criar outra (o onboarding da nova empresa será executado em seguida).

## Exemplo prático

> Ana foi contratada pela imobiliária e recebeu o convite por e-mail. Ela clica
> em **Aceitar convite**, informa nome e senha e entra direto no workspace da
> empresa. No primeiro acesso, conclui o onboarding com o CNPJ, o logotipo e o
> convite do Bruno como gestor. A partir daí, o menu lateral aparece completo e
> ela começa a cadastrar leads.

## Dicas

- Esqueceu a senha? Use **Esqueci minha senha** na tela de login: você recebe
  um link de recuperação por e-mail.
- O convite é pessoal e intransferível: ele vincula seu e-mail ao workspace.
- Cada empresa é um ambiente isolado — dados de um workspace não aparecem no outro.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin` | Cadastra novas empresas, convida membros e conclui o onboarding. |
| demais | Entram por convite; o admin define as permissões de cada um. |

## Veja também

- [Equipe e Permissões](./15-equipe.md) — convidar membros e definir perfis.
- [Configurações e Assinatura](./16-configuracoes.md) — plano e identidade visual.
- [Perguntas frequentes](./20-faq.md)
