# DIMOB — conformidade e geração do arquivo de importação

A DIMOB (Declaração de Informações sobre Atividades Imobiliárias) é a
obrigação acessória anual do setor imobiliário perante a Receita Federal,
instituída pela IN RFB nº 1115/2010 e detalhada nas Perguntas e Respostas
oficiais. A Suitee gera o arquivo de importação no leiaute oficial do
programa da RFB a partir dos repasses de locação do ano-base. Este documento
consolida a base legal, os regimes contábeis, as regras de valoração e de
leiaute aplicadas pelo produto, as validações e as pendências conhecidas.

Fontes: manual operacional e parecer técnico anexados (estudos de consultoria
DIMOB), código em `toca-financial/internal/dimob` (leiaute e validação) e
`toca-financial/internal/services/fiscal_service.go` (montagem e
enriquecimento).

## 1. Base legal e obrigatoriedade

A IN RFB 1115/2010 obriga as pessoas jurídicas (e equiparadas) que realizam
operações imobiliárias específicas, em quatro pilares:

1. **Comercialização de imóveis próprios** — venda/revenda de imóveis
   construídos, loteados ou incorporados para comercialização.
2. **Intermediação imobiliária** — corretagem de compra, venda ou locação de
   imóveis de terceiros.
3. **Sublocação** — sublocação de imóveis a terceiros.
4. **PJ imobiliária dedicada** — pessoa jurídica constituída especificamente
   para construir, administrar, locar ou alienar imóveis próprios, de
   condomínios ou de terceiros.

**Pessoa física equiparada:** a PF que executa loteamento com **mais de 13
lotes** é equiparada a PJ e fica obrigada à DIMOB.

**Quem NÃO é obrigado** (dispensado de entregar):

- **Sem movimento:** PJ com CNAE imobiliário que não realizou nenhuma
  operação reportável no ano — a RFB não aceita declaração sem movimento.
- **Gestão limitada:** atuação restrita a limpeza, segurança e manutenção,
  sem administração de locação ou de receitas.
- **Venda isolada de ativo:** venda pontual de imóvel próprio por empresa sem
  atividade imobiliária no objeto social/CNAE.
- **Revenda pura:** compra de imóveis prontos de terceiros para revenda
  (estoque comercial), sem incorporação, construção ou intermediação.
- **PF direta:** pessoa física que loca ou vende seus próprios imóveis, sem
  ultrapassar o limite do loteamento.
- **Aquisição sem operação:** compra de imóvel no ano sem venda,
  desenvolvimento ou locação subsequentes.

## 2. Regimes contábeis: competência vs. caixa

A DIMOB separa as operações em dois regimes, independentemente do regime
tributário da empresa (Lucro Real ou Lucro Presumido):

| Operação | Regime | Gatilho |
|---|---|---|
| Construção / incorporação / loteamento | Competência | data de assinatura do contrato (valor total no ano da assinatura) |
| Intermediação de venda | Competência | data do contrato de compra e venda |
| Locação / sublocação | Caixa | mês do efetivo recebimento |
| Intermediação de locação | PF: caixa / PJ: competência | Perguntas e Respostas oficiais, #22 |

**Cenários dos manuais** (manual operacional e parecer técnico):

- **Cenário A (construção):** contrato assinado em 15/03/2025 → o valor total
  entra na DIMOB 2025 (entregue em fevereiro de 2026), mesmo com parcelas
  recebidas em anos seguintes.
- **Cenário B (loteamento):** contrato de lote assinado em dezembro/2024 com
  parcelas recebidas em 2025 → pertence à DIMOB 2024, **não** à de 2025.
- **Cenário C (locação):** aluguel de galpão contratado em 2020 com
  pagamentos mensais em 2025 → todos os recebimentos de 2025 entram na DIMOB
  2025, mês a mês.

**Na Suitee**, a locação segue o **regime de caixa**: o arquivo é montado a
partir dos repasses (OwnerTransfer) do ano-base, e o mês do recebimento é o
mês declarado — aluguel bruto por mês e taxa de administração na coluna de
comissão.

## 3. Regras de valoração

A DIMOB informa **fluxos brutos** — não é cálculo de imposto. Conforme o
manual operacional e o parecer técnico:

- **Aluguel bruto:** valor bruto pago pelo locatário, sem deduzir taxa de
  administração, comissões, PIS/COFINS ou IRRF.
- **Juros e multa de mora:** somam ao valor mensal do aluguel (Perguntas e
  Respostas, #21). A taxa de administração retida **sobre os encargos** não
  entra no aluguel declarado.
- **Comissão:** declarada em coluna própria (ficha Locação) — no produto, a
  taxa de administração do repasse preenche essa coluna.
- **IRRF:** coluna própria de imposto retido na fonte, que também **não
  reduz** o aluguel bruto. **Hoje a Suitee grava zero**: a plataforma ainda
  não modela retenção na fonte (ver pendências).

## 4. Leiaute do arquivo gerado

O arquivo `dimob-YYYY.txt` segue o leiaute oficial do programa da RFB:
**largura fixa, CRLF (0D0A), apenas ASCII** e valores com 12 posições de parte
inteira + 2 decimais (centavos). Datas em **DDMMAAAA**. Fichas emitidas hoje:

| Registro | Largura | Conteúdo |
|---|---|---|
| Header | 374 | `DIMOB` + 369 posições **reservadas em branco** (o registro não carrega ano, CNPJ, razão social nem versão: quem declara é o R01) |
| R01 | 270 | Dados da declarante: CNPJ (14), ano (4), retificadora (1), recibo (10), situação especial (1+8+2), nome (60), CPF do responsável (11), endereço (120), UF (2), código de município (4) |
| R02 | 797 | Ficha Locação — uma linha por locação (tabela a seguir) |
| T9 | 102 | Encerramento: `T9` + 100 posições **reservadas em branco** (sem CNPJ e sem contagem de linhas) |

Registro R02 (ficha Locação), campos e posições:

| Campo | Posição | Tam. | Origem na Suitee |
|---|---|---|---|
| Tipo / CNPJ / ano | 1 / 4 / 18 | 3+14+4 | declarante |
| Sequencial da locação | 22 | 5 | ordem do arquivo (1..N) |
| Locador (doc + nome) | 27 / 41 | 14 + 60 | repasse → toca-crm (CPF/CNPJ e nome do proprietário) |
| Locatário (doc + nome) | 101 / 115 | 14 + 60 | toca-deal (contrato) → toca-crm |
| Número do contrato | 175 | 6 | toca-deal (porção numérica do código) — números maiores são truncados pelo começo |
| Data do contrato | 181 | 8 (DDMMAAAA) | toca-deal |
| 12 meses × (aluguel, comissão, imposto) | 189..692 | 14 cada | repasses do mês: aluguel bruto, taxa de administração, IRRF (zero) |
| Tipo do imóvel | 693 | 1 (U/R) | padrão "U" (urbano) |
| Endereço / CEP / município / UF | 694 / 754 / 762 / 786 | 60+8+4+2 | toca-core (rota interna `/api/v1/core/internal/properties/:id`) |

Repasses **cancelados** e repasses **de venda** são ignorados na montagem —
intermediação de venda pertence à ficha R04, ainda não emitida (quando o
chamador passa operações de venda, o gerador devolve advertência em vez de
omitir em silêncio: omissão na DIMOB é infração com multa sobre o valor).

O gerador **confere a largura de cada registro** antes de devolver o arquivo e
recusa datas fora de DDMMAAAA — um caractere a mais desloca todos os campos
seguintes, e um arquivo deslocado é lido errado (ou recusado) pelo programa.
Um R02 com sequencial de 7 posições em vez de 5 (o que a primeira versão fazia)
é exatamente esse caso: tudo depois do sequencial anda dois caracteres.

Fontes do leiaute: tabela F661OIM (documentação de mercado que reproduz o
leiaute do programa, inclusive a observação de 6 posições para o número do
contrato), convenções de formato do leiaute DIMOB (R$, DATA, ANO, X, N, CPF,
CNPJ, CPF/CNPJ, EOL) e as implementações de referência de mercado
(SpedBr/fiscalbr-net), que trazem as posições absolutas de cada campo e os
registros de header e T9.

### Conferência do arquivo (`cmd/validate-dimob`)

Antes de importar, o arquivo pode ser conferido posicionalmente — é o mesmo
olhar do programa da RFB:

```
go run ./cmd/validate-dimob -file dimob-2026.txt        # relatório campo a campo
curl -s .../fiscal/dimob/2026/file | go run ./cmd/validate-dimob -file -
```

O relatório marca `[ERRO]` (o programa recusa ou lê errado) e `[AVISO]` (o
programa aceita, mas o dado compromete a declaração): documentos das partes e
número do contrato em branco, código de município ausente, UF ausente. Sai com
código 1 quando há erro — dá para usar em checklist de entrega.

## 5. Validações do produto e fluxo no programa oficial

A geração aplica as regras do programa da RFB em duas camadas.

**Bloqueantes** (a geração/download falha com erro 400 explicando o motivo):

- CNPJ da declarante sem 14 dígitos.
- Nome empresarial (razão social) ausente.
- Endereço ou CEP do imóvel ausentes/inválidos — a mensagem **cita cada
  contrato pendente**.
- Documento de contraparte (locador/locatário) com formato inválido (CPF 11
  ou CNPJ 14 dígitos).
- Declaração retificadora sem o número do recibo da declaração substituída.
- **Declaração sem movimento** — a RFB não aceita; a geração é bloqueada.

**Advertências** (não bloqueiam; expostas no header `X-Dimob-Warnings` do
download):

- Contrato sem número.
- Contrato sem data.
- Código IBGE do município ausente.

**Fluxo no programa oficial:** abrir o programa/validador da DIMOB →
**Declaração → Importar** → selecionar `dimob-YYYY.txt` → conferir as fichas
→ **Gravar** → **Transmitir** pela internet (Receitanet) → guardar o **recibo
de transmissão**.

## 6. Pendências e roadmap

| Tema | Regra | Status no produto |
|---|---|---|
| Fichas R03/R04 | Construção/incorporação/loteamento e intermediação de venda seguem **competência** (valor na assinatura do contrato) | Estruturas existem no pacote `dimob`, mas o serviço **não emite** — depende de listagem de contratos de venda |
| IRRF | Coluna própria de imposto retido na fonte; não reduz o aluguel bruto | Sempre **zero** — a plataforma ainda não modela retenção na fonte |
| CPF/CNPJ das partes (R02) | Campos obrigatórios da ficha (locador e locatário) | Sai **em branco** quando o lead não tem documento no CRM nem o contrato tem signatário com documento — o programa aceita com advertência, mas é informação omitida para a Receita |
| Código do município | Campo de 4 dígitos no R01/R02, na tabela do próprio programa (Anexo A das instruções de importação) — **não** é o código IBGE de 7 dígitos | Não preenchido — só CEP/UF (vira advertência no programa e no `validate-dimob`) |
| Cadastro da declarante (R01) | Endereço/UF e CPF do responsável perante a RFB | Saem **em branco** — o cadastro da empresa não tem esses campos |
| Centralização matriz/filiais | DIMOB é centralizada na matriz; filiais não entregam declaração própria | Não consolidado — a geração é por tenant |
| Situações especiais | Extinção, fusão, incorporação e cisão antecipam o prazo para o último dia útil do mês seguinte ao evento | Leiaute suporta os campos, mas o serviço não os preenche |
| Não residentes | Exigem CPF/CNPJ brasileiro (PF/PJ estrangeira deve se registrar na RFB) | Validação de formato CPF/CNPJ já aplicada às contrapartes; sem tratamento específico |

## 7. Penalidades e checklist pré-envio

**Penalidades** (IN RFB 1115/2010):

- **Atraso na entrega:** R$ 500/mês ou fração no primeiro ano de atividade
  (início de atividade); R$ 1.500/mês ou fração para as demais PJs.
- **Dados inexatos/omitidos:** 3% do valor da operação inexata ou omitida,
  com mínimo de R$ 100 — aplicada em autuação fiscal (cruzamentos com
  cartórios, bancos e IR), não automaticamente na transmissão.
- **Retificação espontânea:** corrigir antes de qualquer intimação elimina ou
  mitiga a multa de inexatidão.
- **Dolo:** informação falsa ou simulação pode caracterizar crime contra a
  ordem tributária.

**Prazo:** último dia útil de fevereiro do ano seguinte ao ano-base.

**Checklist pré-envio:**

- [ ] Confirmar que a empresa está obrigada (4 pilares) e tem movimento no
      ano-base.
- [ ] CNPJ (14 dígitos) e razão social corretos na geração.
- [ ] Endereço e CEP de todos os imóveis locados completos no cadastro.
- [ ] Contratos com número e data (advertências revisadas no
      `X-Dimob-Warnings`).
- [ ] Rodar `go run ./cmd/validate-dimob -file dimob-YYYY.txt` e resolver os `[ERRO]` antes de importar.
- [ ] Conferir aluguel bruto × comissão por mês após importar no programa.
- [ ] Gravar, transmitir e guardar o recibo de transmissão.
- [ ] Reentregar como retificadora informando o recibo anterior, quando
      houver correção.
