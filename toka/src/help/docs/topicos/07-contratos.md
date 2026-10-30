---
title: Contratos e Documentos
summary: Modelos, contratos, assinatura eletrônica e documentos.
tags: [contrato, modelo, assinatura, clicksign, documento, superdoc]
roles: [admin, gestor, corretor, operacoes, financeiro]
---

# Contratos e Documentos

## O que é

A área que concentra o ciclo de contratação da imobiliária: modelos
editáveis, contratos gerados na esteira, assinatura eletrônica via Clicksign
e o repositório único de documentos.

## Onde fica

Menu **Portfólio & Negócios**:

- **Contratos** → `/contratos`
- **Modelos de Contrato** → `/modelos-contrato`
- **Documentos** → `/documentos`

## Passo a passo: criar um modelo de contrato

1. Em **Modelos de Contrato**, clique em **Novo Modelo**.
2. Escreva o texto no editor SuperDoc (formato DOCX, com formatação completa).
3. Insira as variáveis `{{ }}` (ex.: `{{inquilino_nome}}`,
   `{{valor_aluguel}}`) com **Inserir variável**.
4. Confira o mapeamento: cada variável corresponde a um campo do negócio,
   preenchido automaticamente na geração do contrato.
5. Salve o modelo. Ele fica disponível ao gerar contratos na esteira.

## Passo a passo: enviar um contrato para assinatura

1. Na esteira, avance o negócio até o estágio **Contrato** (veja
   [Esteira Digital](./06-esteira.md)).
2. Abra o contrato em **Contratos** e confira as cláusulas em
   **Conteúdo do Contrato**.
3. Em **Partes Envolvidas (Signatários)**, adicione as partes (proprietário,
   inquilino, testemunhas) e o método de confirmação de cada uma.
4. Revise as parcelas na aba **Financeiro**.
5. Clique em **Enviar para Assinatura** e acompanhe na aba **Assinatura**: o
   status passa por **Em assinatura** e, quando todos assinam na Clicksign,
   vira **Assinado** e depois **Vigente**.

*Print: detalhe do contrato com signatários e parcelas.*

## Lista de contratos

- Busque por código, cliente ou imóvel e filtre por **Rascunho**,
  **Em assinatura**, **Assinados**, **Vigentes** e **Encerrados**.
- Se o envelope expirar (7 dias sem assinatura), o contrato volta para
  **Rascunho**: basta reenviar o convite.

*Print: lista de contratos.*

## Repositório de documentos

1. Em **Documentos**, todos os arquivos da imobiliária ficam centralizados:
   contratos, laudos de vistoria, propostas e comprovantes.
2. Busque por documento, cliente, imóvel, contrato, responsável ou tag e
   filtre por categoria.
3. Use **Visualizar arquivo** para o preview e as tags para organizar.
4. O histórico de quem enviou, validou e assinou fica registrado na trilha
   de auditoria.

## Exemplo prático

> O negócio de Gabriela chega ao estágio **Contrato**. Diego abre o detalhe,
> configura os signatários — Fábio como proprietário, Gabriela como inquilina
> e Carla como testemunha — e clica em **Enviar para Assinatura**. Todos
> assinam pela Clicksign em minutos: o status vira **Assinado** e, em
> seguida, **Vigente**, com o PDF salvo no repositório de documentos.

## Dicas

- Prefira variáveis a textos fixos: o contrato sai preenchido com os dados
  reais do negócio.
- Confira signatários e parcelas antes de **Enviar para Assinatura**; depois
  do envio, o envelope é o documento oficial.
- Anexe o laudo de vistoria ao contrato pela aba **Documentos** (veja
  [Locação e Manutenção](./08-locacao.md)).
- Assinatura pendente expira em 7 dias: reenvie o convite a partir da aba
  **Assinatura**.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `gestor`, `operacoes` | Criar e editar modelos e contratos, enviar para assinatura, excluir e validar documentos. |
| `corretor` | Ver contratos e documentos; editar documentos. |
| `financeiro` | Ver contratos e documentos; validar documentos e assinar como signatário. |
| `auditor` | Somente leitura, com acesso à trilha de auditoria. |

## Veja também

- [Esteira Digital](./06-esteira.md)
- [Locação e Manutenção](./08-locacao.md)
- [Financeiro](./11-financeiro.md) — parcelas e cobranças do contrato.
