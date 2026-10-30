---
title: Portal do Cliente
summary: Portal do cliente e do proprietário: login OTP, imóveis, negócios, locação e financeiro.
tags: [portal, cliente, proprietário, otp, login portal]
roles: [cliente, parceiro]
---

# Portal do Cliente

## O que é

O Portal é a área exclusiva de clientes, inquilinos e proprietários. Nele a
pessoa acompanha o negócio, assina contrato, paga parcelas, abre chamados e
conversa com a imobiliária — sem precisar acessar a área interna do app.

## Onde fica

- **Login do portal**: rota `/portal/login`.
- **Dashboard**: rota `/portal`.
- Demais telas: `/portal/imoveis`, `/portal/buscar`, `/portal/deals/:id`,
  `/portal/locacao`, `/portal/finance`, `/portal/chamados`,
  `/portal/suporte` e `/portal/perfil`.

## Passo a passo: entrar no portal

1. Acesse `/portal/login` e escolha o método **E-mail** ou **WhatsApp**.
2. Informe o e-mail ou o número cadastrado e clique em **Enviar Código**.
3. Digite o código de 8 dígitos recebido — o acesso é confirmado
   automaticamente.
4. Você cai no dashboard `/portal`, com a saudação, o estágio atual do seu
   negócio e os lembretes pendentes.

*Print: dashboard do portal do cliente.*

## Passo a passo: acompanhar um negócio

1. No dashboard, clique no botão do estágio (ex.: **Ver proposta**).
2. Em `/portal/deals/:id`, veja a proposta atual: aceite ou envie uma
   contraproposta com valor e motivo.
3. Na fase de análise, suba os documentos pedidos no checklist e acompanhe a
   aprovação de cada item.
4. Na fase de contrato, assine eletronicamente no painel de assinatura.
5. Com tudo assinado, baixe o contrato em PDF.

## Imóveis, locação e financeiro

- **Imóveis** (`/portal/imoveis`): seus imóveis e locações; em `/portal/buscar`,
  pesquise o catálogo, favorite imóveis e agende visitas.
- **Locação** (`/portal/locacao`): contratos ativos com parcelas e recibos.
- **Financeiro** (`/portal/finance`): boletos, pagamento via PIX, 2ª via e
  histórico de pagamentos — e, nas cobranças vencidas, multa e juros
  discriminados com o valor original, o total e os dias de atraso.

*Print: catálogo de imóveis do portal.*

## Chamados, suporte e perfil

- **Chamados** (`/portal/chamados`): abra solicitações de manutenção com fotos
  e acompanhe o andamento.
- **Suporte** (`/portal/suporte`): dúvidas e canais de contato da imobiliária.
- **Perfil** (`/portal/perfil`): edite dados pessoais e bancários, e baixe
  seus dados (veja [Privacidade e LGPD](./19-seguranca-lgpd.md)).

## Exemplo prático

> Gabriela recebe o convite da Imobiliária Lar Bom, entra no portal por OTP e
> vê o negócio no estágio Proposta. Ela aceita a proposta de R$ 3.500/mês,
> envia RG e comprovantes no checklist e assina o contrato — junto com Fábio,
> o proprietário, e a testemunha Carla. Com o contrato ativo, Gabriela paga a
> primeira parcela via PIX em `/portal/finance` e abre um chamado de
> manutenção para o chuveiro, anexando duas fotos.

## Dicas

- O código OTP chega por e-mail ou WhatsApp — confira a caixa de spam se
  demorar.
- Documento recusado? Leia o motivo na tela, envie a versão corrigida e o item
  volta para revisão.
- Proprietário? Em `/portal/locacao` você vê o resumo de cada imóvel e, no
  financeiro, os repasses e o informe anual para o IR.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `cliente` | Negócios, imóveis, locação, boletos, chamados e perfil. |
| `parceiro` | O mesmo, com visão de portfólio quando tem vários imóveis. |
| demais perfis | Não acessam o portal; usam o app interno. |

## Veja também

- [Contratos e Assinatura](./07-contratos.md)
- [Locação](./08-locacao.md)
- [Privacidade e LGPD](./19-seguranca-lgpd.md)
