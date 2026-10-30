---
title: Privacidade e LGPD
summary: Privacidade, exclusão e exportação de dados (LGPD).
tags: [lgpd, privacidade, exclusão, exportação, consentimento]
roles: [admin, gestor, operacoes, auditor]
---

# Privacidade e LGPD

## O que é

A Suitee trata dados pessoais de clientes, inquilinos, proprietários e da
equipe. Este tópico resume os fluxos de privacidade: consentimento de
marketing, exportação de dados do titular e pedidos de exclusão, conforme a
LGPD.

## Onde fica

- **Titular (portal)**: card **Privacidade — Lei Geral de Proteção de Dados**
  em `/portal/perfil`, com o botão **Baixar meus dados (JSON)**.
- **Titular**: use as ferramentas do perfil ou o canal de privacidade
  informado na Política de Privacidade. Para dados inseridos por uma
  imobiliária, a imobiliária também é um canal de atendimento.
- **Operação**: registre o pedido, confirme a identidade e encaminhe ao
  responsável pelo tratamento. Consulte **Administração → Audit Trail** quando
  houver registro disponível.

## Consentimento de marketing

- O titular controla a autorização de receber comunicações de marketing nas
  preferências do perfil do portal.
- A imobiliária só deve enviar campanhas para quem consentiu; a revogação vale
  a partir do momento em que é registrada.

## Exportação de dados do titular

1. O titular abre `/portal/perfil` e clica em **Baixar meus dados (JSON)**.
2. O arquivo inclui dados de conta e preferências, além de referências para
   dados mantidos em outros serviços. Não presuma que seja uma exportação
   integral de todos os sistemas.
3. Se o titular pedir dados adicionais, localize os sistemas e encaminhe a
   solicitação aos controladores responsáveis dentro do prazo legal.

## Passo a passo: atender um pedido de exclusão

1. Receba o pedido pelo Portal, canal de privacidade informado na Política ou
   chamado que permita contato com o titular.
2. Confirme a identidade do titular antes de qualquer ação.
3. Valide com o responsável legal se o pedido se enquadra no direito de
   eliminação — em contratos vigentes, parte dos dados pode ter que
   permanecer.
4. Avalie a base legal, retenções necessárias e os domínios alcançados antes
   de executar a anonimização ou outra medida cabível.
5. Confira o resultado por serviço. Se houver falha parcial, registre-a e
   conclua o atendimento com o responsável técnico.
6. Responda ao titular com o que foi atendido e o que permaneceu, com a
   justificativa aplicável.

## Prazos e trilha de auditoria

- **Prazo**: use os prazos legais aplicáveis ao direito solicitado. A declaração
  completa de acesso observa o prazo do art. 19, § 2º, da LGPD.
- **Trilha**: registre o recebimento, a análise, as ações por serviço e a
  resposta ao titular conforme os mecanismos de auditoria disponíveis.
- **O que permanece**: preserve somente os dados necessários quando houver
  obrigação legal, exercício regular de direitos ou outra base aplicável; não
  prometa anonimização ou eliminação imediata em todos os serviços.

## Dicas

- Nunca exclua dados com base apenas em ticket genérico: confirme a
  identidade do titular primeiro.
- Dados sensíveis aparecem mascarados no Audit Trail e nas exportações.
- Em caso de dúvida, registre o pedido e consulte o responsável legal antes de
  executar.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `gestor` | Acompanhar pedidos de privacidade e a trilha de auditoria. |
| `operacoes` | Receber e executar pedidos de exportação e exclusão. |
| `auditor` | Conferir a trilha e as evidências do atendimento. |
| `cliente`, `parceiro` | Exportar os próprios dados e revogar consentimento no portal. |

## Veja também

- [Portal do Cliente](./18-portal-cliente.md)
- [Equipe, Permissões e Auditoria](./15-equipe.md)
- [Perguntas frequentes](./20-faq.md)
