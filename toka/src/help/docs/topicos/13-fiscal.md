---
title: Fiscal e Relatórios
summary: DIMOB, NFS-e, informes de rendimentos e relatórios.
tags: [fiscal, dimob, nfs-e, informe, relatório, exportação]
roles: [admin, operacoes, financeiro, auditor]
---

# Fiscal e Relatórios

## O que é

O Fiscal emite os documentos oficiais da imobiliária — DIMOB, NFS-e de
administração e informes de rendimentos — e os Relatórios consolidam a operação
comercial e financeira em painéis exportáveis.

## Onde fica

- **Fiscal**: menu **Financeiro → Fiscal**, rota `/fiscal`.
- **Relatórios**: menu **Inteligência & Expansão → Relatórios**, rota
  `/relatorios`.

*Print: painel fiscal com DIMOB e informes.*

## Passo a passo: gerar e baixar um documento fiscal

1. Em **Fiscal**, escolha o documento: **DIMOB**, **NFS-e de administração** ou
   **Informe de Rendimentos**.
2. Para NFS-e, clique em **Ir para Emissão**, confirme o serviço (ex.:
   **Intermediação Imobiliária**) e o valor, e emita.
3. Para informes, clique em **Gerar Informe** e aguarde a geração. Para a
   DIMOB, siga o passo a passo a seguir: o CNPJ e a razão social da
   imobiliária são informados na geração.
4. Depois de gerado, clique em **Baixar** e salve o arquivo.

O download exige sessão autenticada e é baixado pelo navegador: abra o link na
mesma janela em que você está logado na Suitee.

## Passo a passo: gerar e importar a DIMOB

1. Em **Fiscal**, clique em **Gerar DIMOB** e informe o **CNPJ** (14 dígitos)
   e a **razão social** da imobiliária — são os dados da declarante no
   arquivo.
2. Confira o resumo das locações do ano e clique em **Baixar** para salvar o
   arquivo `dimob-2025.txt` (o nome usa o ano-base).
3. Abra o **programa/validador da DIMOB** da Receita Federal e vá em
   **Declaração → Importar**.
4. Selecione o arquivo `dimob-2025.txt` e confira as fichas importadas
   (Locação).
5. Clique em **Gravar** e depois em **Transmitir** pela internet (Receitanet)
   para receber o **recibo de transmissão**.

O que é obrigatório para o arquivo ser gerado:

- **CNPJ e razão social** — sem eles, a geração pede os dados novamente.
- **Locação no ano-base** — a Receita **não aceita declaração sem movimento**;
  sem locação no ano, a geração é bloqueada e não há DIMOB a entregar.
- **Endereço e CEP de cada imóvel locado** — sem eles o programa da Receita
  bloqueia a importação. A Suitee bloqueia antes e a mensagem de erro
  **aponta o contrato pendente** (ex.: "contrato CTR-0001: endereço/CEP do
  imóvel pendentes"). Complete o cadastro do imóvel e baixe novamente.

Contratos **sem número ou sem data** não impedem a geração (o programa da
RFB aceita com aviso): o arquivo sai com as advertências no cabeçalho
`X-Dimob-Warnings` do download.

## Passo a passo: exportar um relatório

1. Em **Relatórios**, escolha o relatório: conversão, ranking de agentes,
   comissões geradas, resumo financeiro, entre outros.
2. Ajuste o período e os filtros.
3. Clique em **Exportar** e escolha CSV ou PDF.
4. Salve o arquivo baixado.

## Exemplo prático

> No fim do ano, Eliane precisa do informe do proprietário Fábio e da DIMOB da
> imobiliária. Em **Fiscal**, ela gera o **Informe de Rendimentos** de Fábio e
> o envia pelo portal. Depois clica em **Gerar DIMOB**, informa o CNPJ e a
> razão social da imobiliária e baixa o arquivo `dimob-2025.txt` com as
> locações do ano. Ao importar no programa da Receita, o validador acusa falta
> de endereço no apartamento da Rua das Flores: a mensagem aponta o contrato,
> Eliane completa o CEP no cadastro do imóvel, baixa o arquivo de novo e a
> importação passa. Ela grava, transmite e guarda o recibo de transmissão.
> As vendas do ano (como o apartamento de R$ 800 mil, com NFS-e de
> intermediação de R$ 48 mil já emitida) ainda não entram no arquivo da DIMOB:
> por enquanto a Suitee declara as locações.

## Dicas

- Emita a NFS-e da comissão assim que o recebimento for baixado.
- Use o **Calendário Fiscal** para acompanhar as obrigações por período.
- O **Histórico de Relatórios** guarda as últimas gerações para rebaixar.
- A DIMOB é anual: gere depois do fechamento do ano e entregue até o último
  dia útil de fevereiro.
- Complete endereço e CEP dos imóveis **antes** de gerar a DIMOB — é o
  bloqueio mais comum na importação.
- Sem locação no ano, não entregue DIMOB: a Receita não aceita declaração sem
  movimento.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `operacoes`, `financeiro` | Gerar e baixar documentos fiscais e exportar relatórios. |
| `gestor` | Ver o painel fiscal e exportar relatórios. |
| `auditor` | Ver tudo e exportar relatórios; não gera documentos. |

## Veja também

- [Financeiro](./11-financeiro.md) — recebimentos que alimentam o fiscal.
- [Locação](./08-locacao.md) — contratos que compõem a DIMOB.
