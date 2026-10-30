---
title: Inadimplência
summary: Régua de cobrança, cobranças atrasadas e ações de recuperação.
tags: [inadimplência, atraso, cobrança, régua, recuperação]
roles: [admin, operacoes, financeiro]
---

# Inadimplência

## O que é

A Recuperação acompanha as cobranças atrasadas de locação e venda e organiza a
cobrança em etapas: régua automática de avisos, contatos registrados,
negociação de dívida, acionamento de garantias e, no limite, ação judicial.

## Onde fica

Menu **Financeiro → Recuperação**, rota `/inadimplencia`.

*Print: cobranças atrasadas com régua de ações.*

## Como a régua de cobrança funciona

A régua notifica o cliente automaticamente por e-mail e WhatsApp: lembrete 3
dias antes do vencimento, no dia do vencimento e, depois do atraso, avisos com
juros e multa em D+1, D+5, D+15 e D+30. Qualquer pagamento interrompe a régua
sozinho.

A tela abre com os indicadores calculados pelo servidor — **Total em Atraso**,
**Em Cobrança**, **Garantias Acionadas**, **Processos Judiciais** — e o **aging**
da carteira por faixa de atraso (0–30, 31–60, 61–90, 90+). Esses números são do
total da carteira: filtrar a tabela ou buscar um inquilino não muda o que está no
topo. A tabela mostra contrato, inquilino, valor em atraso, dias de atraso e
status, com paginação do servidor (20 casos por página) e as abas
**Inadimplentes**, **Cobrança**, **Garantias** e **Judicial**, que filtram de
verdade por status.

O caso de cobrança é criado automaticamente quando o atraso alcança o **limiar
da régua** — **5 dias** no padrão da plataforma, ou o número que a sua
imobiliária configurar (a tela mostra o limiar vigente e de onde ele vem). Por
isso a aba **Inadimplentes** traz, abaixo da tabela de casos, o bloco
**Cobranças vencidas sem caso aberto**: são as cobranças que já passaram do
vencimento e ainda não viraram caso. É o que torna visível o efeito de
**Processar Inadimplência** nos primeiros dias de atraso.

Cada linha desse bloco tem o botão **Abrir caso**, que antecipa a abertura sem
esperar o limiar — é o "abrir manualmente". Como a ação cria a dívida formal do
cliente (com juros e multa pela régua), o sistema pede confirmação antes de
enviar e mostra a mensagem do servidor quando a cobrança já está em um caso
aberto ou quando os valores não conferem; nesses casos nada é criado.

A cobrança vence **no dia** do vencimento e só conta atraso **no dia seguinte**:
o rótulo mostra *Vence hoje* no dia do vencimento, *Venceu ontem* em D+1 e
*Venceu há N dias* daí em diante (cobrança paga ou cancelada nunca aparece como
atrasada).

## Passo a passo: registrar uma cobrança

1. Na tabela, localize a linha do contrato pelo inquilino ou pelo contrato (a
   busca filtra os casos da página carregada).
2. Clique no ícone de telefone (**Procedimento de Cobrança**).
3. No painel, escolha o tipo (**Ligação Telefônica**, **Mensagem WhatsApp**,
   **E-mail** ou **Notificação Extrajudicial**) e descreva a resposta obtida.
4. Clique em **Registrar Procedimento**. A ação entra na timeline do caso — no
   painel e na aba **Cobrança** — com data, autor e descrição.

## Passo a passo: fechar um acordo

1. Clique em **Acordo de Dívida** na linha do cliente que propôs o parcelamento.
2. Informe o **valor do acordo** (já vem preenchido com a dívida do caso), o
   **desconto**, a **entrada** (se houver), o **número de parcelas** (1 a 36) e o
   **vencimento da primeira parcela**. Em *opções avançadas* você ajusta o
   intervalo entre parcelas (30 dias por padrão).
3. Confira a prévia (“4x de R$ 2.125,00 + entrada de R$ 500,00, primeira em …”).
4. Clique em **Registrar Acordo**. O sistema **cancela as cobranças originais** e
   cria uma cobrança para cada parcela — por isso o aviso na tela antes de
   confirmar: essa troca não se desfaz pela tela. O caso passa para **Negociado**.
5. Acompanhe as parcelas no próprio painel: cada uma tem sua cobrança, com
   vencimento e status. O acordo é marcado como **Cumprido** quando todas são
   pagas.
6. Se o devedor não cumprir, use **Quebrar Acordo**: informe o motivo e o caso
   volta para **cobrança ativa**. A opção *Restabelecer o desconto* (desmarcada
   por padrão) recria o desconto concedido como cobrança nova — marque apenas se a
   imobiliária cobra o valor cheio quando o acordo é descumprido.

> Acordo com desconto acima do limite de alçada exige aprovação de quem tem a
> permissão de ajuste financeiro; sem ela o sistema recusa e explica o motivo.

## Escalando a recuperação

- **Acionar Garantia**: escolha a modalidade (**Seguro Fiança**, **Fiador** ou
  **Depósito Caução**), informe o valor a acionar e o motivo. O painel mostra o
  protocolo, a seguradora e a resposta do garantidor; use **Registrar Resposta**
  para atualizar o status do acionamento (aberto, aceito, recusado ou cancelado).
- **Registrar Indenização Recebida**: quando o garantidor paga, informe o valor
  recebido e a referência (protocolo do sinistro, depósito judicial). O sistema dá
  baixa nas cobranças do caso **da mais antiga para a mais nova**, transforma o que
  sobrar em crédito do caso e encerra o caso quando a dívida é quitada. No
  **Depósito Caução** o valor sai da caução do contrato e o acerto da rescisão
  passa a descontar o que foi usado. Se o garantidor recusar ou cancelar, o caso
  volta para cobrança ativa e a régua de avisos retoma.
- **Processo Judicial**: informe o **número do processo no padrão CNJ**
  (0000000-00.0000.0.00.0000 ou os 20 dígitos) e a **vara/comarca** — os dois são
  obrigatórios —, além do advogado (nome, OAB, e-mail e telefone), o valor da
  causa, o andamento e a data da próxima audiência.
- **Desfecho Judicial**: com o caso em **Judicial**, registre o resultado e a data
  da decisão, e lance as **custas e honorários** (cada item vira uma despesa do
  contrato). O efeito depende do desfecho: **Acordo** leva o caso para
  **Negociado** (para você formalizar o parcelamento), **Sentença Procedente**
  mantém o caso aguardando o recebimento, e **Sentença Improcedente** ou
  **Arquivado** devolvem o caso para **cobrança ativa**, com a régua retomando os
  avisos.
- **Rescisão de Contrato**: o resumo do encerramento usa os dados reais do
  contrato — cobranças em aberto até a data, saldo devedor, cobranças futuras a
  cancelar e a devolução de caução apurada pelo servidor. Quando um dado não está
  disponível, o campo aparece como “—”.

Depois de um processo, use a rescisão de contrato; o imóvel volta ao portfólio
para uma nova locação.

## Exemplo prático

> Gabriela está com a parcela de R$ 3.500 da locação vencida há 3 dias. A régua
> já enviou os avisos e a cobrança aparece na tabela de Eliane como **Em
> Cobrança**. Diego liga e registra a promessa de pagamento até o dia 20. Como
> o prazo não foi cumprido, Eliane abre **Negociar** e registra um acordo de
> R$ 3.300 em 3 parcelas. Gabriela paga as três via PIX, a cobrança é quitada e
> o valor volta ao repasse normal do proprietário Fábio.

## Dicas

- Registre toda tentativa de contato: o histórico sustenta garantia e processo.
- Prefira negociar cedo — um acordo de boa-fé preserva o contrato e pausa a régua.
- Acompanhe o KPI **Total em Atraso** para ver a carteira inteira de uma vez.
- Ao acionar garantia, confira antes o valor coberto pelo seguro ou fiador.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `financeiro` | Registrar cobranças, negociar, acionar garantias e iniciar processos. |
| `operacoes` | Registrar cobranças e conduzir a recuperação no dia a dia. |
| `gestor`, `auditor` | Visualizar a carteira em atraso. |

## Veja também

- [Financeiro](./11-financeiro.md) — origem das cobranças e baixas.
- [Locação](./08-locacao.md) — contratos e parcelas recorrentes.
- [Portal do Cliente](./18-portal-cliente.md) — onde o cliente paga e vê os avisos.
