---
title: Configurações e Assinatura
summary: Perfil, empresa, aparência, integrações e assinatura.
tags: [configurações, perfil, empresa, aparência, integrações, assinatura, plano]
roles: [admin, gestor]
---

# Configurações e Assinatura

## O que é

As configurações concentram os ajustes da empresa e do seu usuário, e a área
de assinatura mostra o plano contratado, o consumo e o histórico de cobranças
da plataforma.

## Onde fica

- **Configurações**: menu **Administração → Configurações**, rota
  `/configuracoes`.
- **Assinatura**: menu **Administração → Assinatura & Consumo**, rota
  `/assinatura`.

## Abas de Configurações

- **Perfil** — seus dados pessoais, cargo e foto.
- **Empresa** — dados da imobiliária: CNPJ, endereço e equipe.
- **Aparência** — logotipo, cores e identidade visual do app.
- **Permissões** — matriz de perfis da empresa (exclusiva do `admin`).
- **Notificações** — preferências de avisos por e-mail e no app.
- **Integrações** — conexões com serviços externos (assinatura eletrônica,
  portais de imóveis, entre outros).
- **Geral** — preferências gerais da empresa.

*Print: configurações da empresa.*

## Passo a passo: personalizar a identidade visual

1. Em **Administração → Configurações**, abra a aba **Aparência**.
2. Envie o logotipo da imobiliária e escolha a cor principal.
3. Salve. A nova identidade passa a valer no app inteiro.

## Passo a passo: acompanhar plano e consumo

1. Abra **Administração → Assinatura & Consumo**.
2. Confira o plano atual, os limites e o consumo do mês.
3. No **Histórico**, veja as faturas anteriores.
4. Para mudar de plano, clique no upgrade e siga as instruções de pagamento.

## Exemplo prático

> Marina é `admin` na Imobiliária Lar Bom e `gestor` na Imóveis Premium. Na
> primeira empresa, ela vê todas as abas de configurações e ajusta o logotipo
> na aba **Aparência**; na segunda, as abas **Permissões** e **Notificações**
> não aparecem para ela, e a assinatura mostra apenas o resumo do plano, sem
> opções de alteração. Cada empresa tem configurações e plano próprios — nada
> vaza de uma para a outra.

## Dicas

- Mudou de cor ou logotipo? A troca vale para todos os usuários da empresa na
  hora.
- Permissões mexem no acesso de todos: altere com cuidado; a mudança fica
  registrada no Audit Trail.
- Dúvidas sobre cobrança do plano? Confira o histórico em
  **Assinatura & Consumo** antes de abrir chamado.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin` | Todas as abas, inclusive Permissões e Integrações; gerencia a assinatura. |
| `gestor` | Abas Perfil, Empresa, Aparência e Geral; vê o resumo da assinatura. |
| `operacoes`, `marketing` | Aba Aparência (e Geral para `operacoes`), conforme a matriz da empresa. |
| demais | Sem acesso, salvo liberação específica da empresa. |

## Veja também

- [Equipe, Permissões e Auditoria](./15-equipe.md)
- [Começando na Suitee](./01-comecando.md)
- [Privacidade e LGPD](./19-seguranca-lgpd.md)
