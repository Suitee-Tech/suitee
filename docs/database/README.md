# Banco de dados da plataforma — visualização DBML

Levantamento em **25/09/2026**, a partir das **89 migrations** de `supabase/migrations/`: **14 schemas, 78 tabelas, 547 colunas, 328 índices e 5 FKs SQL**. O snapshot representa o histórico versionado reconstruído; não confirma quais versões estão aplicadas em produção.

## Comece pela visão geral

Importe **[views/01-overview.dbml](views/01-overview.dbml)** para a primeira leitura: são **9 tabelas e 8 relações**, formando o caminho cliente/imóvel → negócio → contrato → assinatura, cobrança, repasse e comissão. O recorte mostra apenas identificadores e campos usados pelas linhas visíveis.

Para aprofundar, escolha um diagrama independente:

| Visão | Tabelas | Relações | Uso |
| --- | ---: | ---: | --- |
| [01 · Visão geral](views/01-overview.dbml) | 9 | 8 | Entender o fluxo central |
| [02 · Comercial](views/02-commercial.dbml) | 8 | 17 | Contas, leads, imóveis, negócios e assinatura |
| [03 · Cobrança](views/03-billing.dbml) | 7 | 11 | Locações, configurações e cobranças |
| [04 · Repasses e razão](views/04-settlement.dbml) | 9 | 17 | Comissões, repasses, obrigações e lançamentos |
| [05 · Operações](views/05-operations.dbml) | 9 | 19 | Manutenção, fornecedores, vistorias e agenda |
| [06 · Comunicação e IA](views/06-communication.dbml) | 9 | 10 | Identidade, mensagens e conversas de IA |

Os recortes são complementares, não exaustivos. Cada arquivo pode ser importado sozinho, sem depender de filtros ou grupos do editor. Os modelos completos abaixo preservam todas as tabelas e relações do levantamento original.

## Arquivos e visualização

- **[platform.dbml](platform.dbml)** — modelo físico completo das tabelas de aplicação, agrupado por domínio, com o financeiro dividido em cobrança, repasses/comissões, razão/fiscal e configuração. Contém tipos, defaults, nulabilidade, PKs simples/compostas, índices e as cinco FKs reais.
- **[platform.logical.dbml](platform.logical.dbml)** — visão complementar de **37 tabelas**, **79 vínculos lógicos** e as **5 FKs físicas**. Mostra o fluxo CRM → negócio → contrato → financeiro, assinatura, operações, comunicação e IA.
- **[inventory.md](inventory.md)** — responsabilidades dos schemas e inventário de tabelas, colunas, PKs e RLS.
- **[relationships.md](relationships.md)** — origem, campo persistido, destino e fonte de cada vínculo lógico.
- **[sources.json](sources.json)** — revisão Git e hashes SHA-256 das migrations usadas.

Abra o [dbdiagram](https://dbdiagram.io/), crie um diagrama e cole o conteúdo de um dos arquivos `.dbml`. Importe cada arquivo em um diagrama separado; ambos reutilizam nomes de tabelas. Os `TableGroup` permitem navegar por domínio. A sintaxe segue a [documentação oficial DBML](https://dbml.dbdiagram.io/docs/).

Comece pelo recorte de visão geral para entender os fluxos; consulte a física para confirmar a estrutura SQL. **Não exporte a visão lógica como migration**: os campos `json__*` são projeções documentais de JSONB e as linhas `logical_*` não representam constraints existentes.

## Organização visual

Cabeçalhos e grupos compartilham uma paleta consistente: azul para CRM, ciano para imóveis, violeta para contratos/assinatura, verde para cobrança/repasses e laranja para operações. Nomes e numeração dos grupos complementam as cores. Os grupos seguem uma ordem de leitura do negócio até os serviços de apoio.

As linhas `logical_*` usam cinza claro; FKs SQL usam cinza escuro. A legenda permanece no próprio diagrama. As notas de evidência continuam disponíveis nos campos. A apresentação usa [recursos oficiais DBML de cores, grupos, notas e DiagramView](https://dbml.dbdiagram.io/syntax/enrichment-visualization/).

Nos arquivos completos, as seis `DiagramView leitura_*` permitem filtrar conjuntos de tabelas em editores compatíveis. Esses filtros mantêm os campos e as relações entre as tabelas selecionadas; os arquivos em `views/` são mais enxutos, especialmente o overview, que seleciona somente oito relações centrais.

Os arquivos `*.dbml.layout.json` guardam as posições das caixas no formato local existente (`version: 1`, `positions`, IDs `table-schema.tabela`). O layout completo foi reorganizado em faixas de leitura e blocos por domínio; a visão geral tem um arranjo próprio. No visualizador local, mantenha o arquivo de posições ao lado do DBML. Em importações para outros editores, o DBML organiza e estiliza o conteúdo, mas as coordenadas desse arquivo auxiliar podem não ser consumidas e o roteamento das linhas continua a cargo do editor. Para leitura horizontal, disponha CRM/imóveis à esquerda, negócio/contrato ao centro e assinatura/financeiro à direita. Os recortes reduzem a quantidade de conexões que precisam ser organizadas. Cores, grupos e filtros dependem do suporte/plano do visualizador; os arquivos separados continuam utilizáveis sem esses recursos.

## Como interpretar os relacionamentos

O armazenamento combina colunas SQL com agregados serializados em `metadata JSONB`. Por isso, os 79 vínculos lógicos não devem ser confundidos com as cinco chaves estrangeiras do catálogo:

| Origem | Colunas | Destino | Exclusão |
| --- | --- | --- | --- |
| `ai.messages` | `conversation_id` | `ai.conversations.id` | CASCADE |
| `ai.tool_invocations` | `conversation_id` | `ai.conversations.id` | CASCADE |
| `financial.transfer_deduction_claims` | `(tenant_id, transfer_id)` | `financial.transfers.(tenant_id, id)` | RESTRICT |
| `financial.managed_bills` | `(tenant_id, source_transfer_id)` | `financial.transfers.(tenant_id, id)` | RESTRICT |
| `financial.managed_bills` | `(tenant_id, source_charge_id)` | `financial.charges.(tenant_id, id)` | RESTRICT |

Na visão lógica, identificadores usam `text` para facilitar a leitura; os tipos reais, inclusive diferenças entre UUID e TEXT, permanecem na visão física. As relações `>` indicam vários registros de origem apontando para um destino; a presença do campo lógico pode ser opcional. A aplicação deve validar existência, autorização e escopo do tenant.

Pontos relevantes do levantamento:

- `deal.deals.owner_id` e `customer_id` no JSON referem-se a **leads**. A conta do portal é uma projeção associada ao lead; não substitui sua identidade. `crm.leads` persiste campos como `AccountID` e `AssignedBrokerID` com maiúsculas, conforme a serialização e as consultas do repositório.
- `tenant_id` é a chave de isolamento, mas não possui FK para uma tabela central de tenants neste histórico. Nenhuma tabela de tenants foi inventada no DBML. `company_id`, `requester_id`, `target_id`, `payer_tenant` e `payee_tenant` também exigem interpretação de identidade organizacional no código.
- `portal.*` armazena integrações e feeds de portais externos. A experiência do cliente também consome `crm`, `deal`, `comm`, `calendar` e `financial`; ela não equivale apenas ao schema `portal`.
- Cobranças, transações, comissões, repasses, ordens de pagamento e lançamentos do razão são entidades distintas. Vínculos entre elas não provam liquidação bancária ou autorização fiscal. `financial.payments` permanece no modelo físico mesmo com o fluxo V1 congelado; `financial.abacate_webhook_events` foi removida pelo histórico e não aparece.
- `journal_entries.metadata.reversal_of` e `reversed_by` são vínculos de estorno com controles por índices/triggers; não são FKs SQL.
- `signature.envelopes.contract_id`/`deal_id` são vínculos lógicos. `external_envelope_id` pertence ao provedor; webhooks e envelopes não possuem FK entre si.

## Limites da representação

O modelo físico abrange todas as tabelas de aplicação presentes após o replay. O lógico é uma seleção dos vínculos com destino identificável, não uma expansão exaustiva de todos os campos dos agregados.

Arrays e referências polimórficas permanecem nos agregados e não foram convertidos em tabelas fictícias: `deal.deals.metadata.parties`, propostas e signatários; `comm.channels.metadata.participants`; itens/alocações/pagamentos de cobranças; pernas contábeis de `journal_entries`; comentários/anexos de chamados. Campos como `entity_id`, `source_id`, `recipient_id` ou `agent_id` dependem do tipo/contexto e não recebem uma FK presumida. A relação de remetente de mensagem com conta representa mensagens de usuários; identidades de sistema/provedores precisam de tratamento próprio.

Índices parciais e métodos GIN/GiST não são transformados em unicidade global: sua definição SQL integral fica na `Note` da tabela. CHECKs também ficam nas notas. Índices B-tree/hash compatíveis são representados no bloco `indexes`. RLS aparece como informação, não como uma definição de policies; grants, funções e triggers continuam nas migrations. Não use o DBML como substituto de um dump para restauração.

`audit.events` aparece uma vez com sua chave de particionamento; partições mensais dependem da data de execução e foram excluídas da contagem. Schemas internos gerenciados por Supabase (`auth`, `storage`, `realtime`, extensões e ledger de migrations), Redis, filas e object storage não são schemas de aplicação definidos por este histórico e não foram inventariados como tabelas da plataforma.

O bootstrap `scripts/postgres/init.sql` é uma alternativa de compatibilidade local, não foi concatenado com o histórico canônico. As migrations antigas de `toca-core` e `toca-signature`, e o bootstrap de autenticação, foram conferidos como fontes auxiliares; não sobrepõem o estado final das migrations Supabase. Isso evita duplicar tabelas ou reintroduzir definições antigas.

## Método e validação

1. Consulta ao Graphify para orientação, seguida de leitura das migrations e dos modelos/repositórios.
2. Replay das migrations, em ordem de nome, em PostgreSQL 14 descartável e vazio, com fuso UTC. Foram fornecidos papéis de compatibilidade, `pgcrypto`, `uuid-ossp`, funções substitutas `auth.jwt()`/`auth.uid()` e `cron.schedule()`; a criação de `pg_cron` foi substituída apenas na cópia em memória utilizada para documentação.
3. Replay limpo das 89 migrations em um segundo banco descartável: catálogo idêntico ao primeiro. Extração de `pg_catalog` com [catalog.sql](catalog.sql), excluindo partições filhas e schemas internos. A ausência de Storage faz os blocos protegidos por verificações de existência serem ignorados.
4. Geração determinística do modelo físico com [generate_dbml.py](generate_dbml.py), e curadoria da visão lógica com evidências de código.
5. Parse dos arquivos com **`@dbml/core` 10.2.0, parser `dbmlv2`** (necessário para os atributos visuais): 78 tabelas/5 referências na visão física e 37 tabelas/84 referências na lógica. Os seis recortes também passaram no parser. A comparação de exportações SQL antes/depois da estilização confirmou a preservação da estrutura dos dois modelos completos.

Esse procedimento valida o catálogo reconstruído e a sintaxe DBML. As funções substitutas não validam autenticação, execução de cron, políticas de acesso ou comportamento de produção. Nenhuma migration foi aplicada a bancos existentes da plataforma.

## Atualização do modelo físico

Use uma conexão **somente leitura** para um banco já reconstruído com o histórico desejado. O extrator consulta metadados, não dados de usuários:

```sh
psql "$DOCUMENTATION_DATABASE_URL" -X -At -v ON_ERROR_STOP=1 \
  -f docs/database/catalog.sql > /tmp/suitee-catalog.json
python3 docs/database/generate_dbml.py /tmp/suitee-catalog.json
```

Execute da raiz do repositório. A variável deve apontar para o banco escolhido para documentação; não há URI embutida nos arquivos. Revise também o inventário, o manifesto de fontes, as notas de contexto e os vínculos lógicos quando as migrations ou modelos de domínio mudarem. A geração física não atualiza automaticamente essas partes editoriais.

## Regeneração da apresentação

Depois de atualizar o modelo lógico, execute:

```sh
python3 docs/database/layout_dbml.py
```

O script reaplica cores, grupos, legenda e filtros de forma idempotente e regenera os seis recortes e seus arquivos de posições. Essa execução também substitui ajustes manuais de coordenadas pelos arranjos definidos no script. A configuração de grupos e seleções está centralizada nesse arquivo; não edite manualmente os recortes gerados. O gerador físico também aplica essa apresentação automaticamente. As coordenadas foram verificadas contra sobreposição de retângulos reservados (460 px de largura e altura estimada pelos campos); a prévia do editor não foi inspecionada visualmente. A validação sintática usa o parser `dbmlv2`; o parser legado `dbml` não aceita todos os atributos visuais, como cor de relacionamento.
