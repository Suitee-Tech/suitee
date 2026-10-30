# Variáveis de contrato — referência

# Variáveis de contrato

Esta referência lista as variáveis que podem ser usadas nos modelos DOCX de
contrato e explica como cada tipo funciona no painel do editor.

> A tabela é gerada a partir do catálogo do sistema. Use a busca do editor pelo
> código em inglês (por exemplo, `buyer_name`) ou pela descrição.

## O que são

Variáveis são trechos entre `{{ }}` substituídos pelos dados reais do negócio,
do imóvel, das partes, do contrato e da imobiliária no momento da geração do
documento.

- `{{buyer_name}}` → nome do comprador.
- `{{property_title}}` → título do imóvel.
- `{{contract_start_date}}` → data de início.
- `{{company_name}}` → nome da imobiliária.

## Onde usar

As variáveis são inseridas em **Modelos de Contrato**, no editor DOCX. O mesmo
catálogo aparece no painel **Variáveis** e é aplicado ao gerar o contrato na
esteira.

Veja também `docs/features/contratos/` (Contratos e Documentos) e
`docs/features/esteira/` (Esteira Digital).

## Sintaxe

| Recurso | Exemplo | Uso |
| --- | --- | --- |
| Valor simples | `{{buyer_name}}` | Um dado escalar. |
| Caminho de objeto | `{{buyer.name}}` | Campo de um objeto. |
| Lista | `{{buyers}}` | Lista de nomes quando usada como valor. |
| Loop | `{{#buyers}}{{name}} — {{document}}{{/buyers}}` | Repete o trecho para cada item. |
| Índice de lista | `{{buyers.0.name}}` | Acessa um item específico; use com moderação. |
| Data calculada | `{{current_date_plus_30_days}}` | Calcula a partir da data atual. |
| Data por extenso | `{{contract_start_date_written}}` | Data já formatada em português. |

## Como inserir no modelo

1. Abra **Modelos de Contrato** e edite um modelo.
2. No painel **Variáveis**, busque pelo código ou pela descrição.
3. Arraste a variável para o documento ou clique para copiar e cole no ponto
   desejado.
4. Use **Criar Variável** quando precisar calcular uma data a partir de um
   texto selecionado.
5. Salve e gere um preview na esteira para conferir os valores.

O painel mostra **Sistema** (catálogo) e **No documento** (o que já foi
detectado). Uma variável desconhecida no documento aparece como pendente para
ser mapeada.

## Objetos e listas

- Objetos usam ponto: `{{buyer.name}}`, `{{landlord.document}}`.
- Listas usam loop: `{{#signers}}{{name}} — {{status}}{{/signers}}`.
- `sellers` e `landlords` existem para contratos com mais de um vendedor ou
  locador.
- `signers` lista os signatários com nome, email, papel, status, data e
  obrigatoriedade.
- `parties` reúne todas as partes cadastradas.

Campos opcionais sem informação são preenchidos com uma linha de preenchimento;
revise no preview antes de enviar para assinatura.

## Datas

Além das datas base, o sistema aceita expressões calculadas:

- `{{current_date_plus_30_days}}`
- `{{current_date_plus_5_business_days}}`
- `{{contract_start_date_plus_12_months_written}}`
- `{{current_date_minus_1_years}}`

Use `_iso` para `AAAA-MM-DD` e `_written` para por extenso. Dias úteis excluem
apenas sábados e domingos; feriados não são considerados.

## Empresa, partes e contrato

- `{{company_name}}`, `{{company_legal_name}}`, `{{company_document}}`,
  `{{company_email}}`, `{{company_phone}}` e `{{company_address}}` vêm da
  empresa logada.
- `{{sellers}}` e `{{landlords}}` repetem o trecho para cada parte do papel.
- `{{signers}}` lista os signatários do envelope.
- `{{contract_*}}` traz dados do contrato; quando o contrato ainda não foi
  salvo, alguns campos ficam vazios.

## Solução de problemas

| Sintoma | Causa provável | O que fazer |
| --- | --- | --- |
| “Nenhuma variável detectada” | Tag em cabeçalho/rodapé ou editor ainda carregando | Clique em reexaminar; salve e reabra o modelo; confira o DOCX. |
| Variável aparece como pendente | Código não existe no catálogo | Use a busca do painel e mapeie para uma variável do sistema. |
| Documento mostra `{{variavel}}` | Variável sem dado e sem preenchimento manual | Preencha no preview ou ajuste o dado de origem. |
| Linha de preenchimento (`______`) | Campo opcional sem informação | Preencha no preview ou deixe o dado de origem. |
| Loop não repete | Lista vazia ou tag de fechamento diferente | Confira `{{#lista}}...{{/lista}}` e os dados da parte. |
| Data vazia | Falta a data base ou a expressão é inválida | Verifique a data do contrato/negócio e a expressão. |

## Compatibilidade

- Variáveis antigas continuam aceitas; algumas têm aliases, como
  `broker_captador_name` para `broker_listing_name`.
- Campos locais dentro de um loop não precisam existir no catálogo global.
- A inspeção do DOCX cobre corpo, cabeçalho, rodapé, notas e comentários.
- Em modelos com muitas variáveis, use a busca e as categorias recolhíveis.

## Tabela completa de variáveis

Código entre `{{ }}`. Campos opcionais viram linha de preenchimento quando o dado não existe no negócio.

### Pessoas (45)

| Variável | O que entrega | Exemplo |
|---|---|---|
| `{{buyer_name}}` | Nome do Comprador | — |
| `{{buyer_document}}` | CPF/CNPJ do Comprador | — |
| `{{buyer_email}}` | Email do Comprador | — |
| `{{buyer_phone}}` | Telefone do Comprador | — |
| `{{buyer_address}}` | Endereço do Comprador — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer}}` | Comprador (Objeto) — Acesse os campos da parte principal com ponto. | `{{buyer.name}} — {{buyer.document}}, {{buyer.address}}` |
| `{{buyers}}` | Compradores (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#buyers}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/buyers}}` |
| `{{tenant_name}}` | Nome do Inquilino | — |
| `{{tenant_document}}` | CPF/CNPJ do Inquilino | — |
| `{{tenant_email}}` | Email do Inquilino | — |
| `{{tenant_phone}}` | Telefone do Inquilino | — |
| `{{tenant_address}}` | Endereço do Inquilino — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant}}` | Inquilino (Objeto) — Acesse os campos da parte principal com ponto. | `{{tenant.name}} — {{tenant.document}}, {{tenant.address}}` |
| `{{tenants}}` | Inquilinos (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#tenants}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/tenants}}` |
| `{{seller_name}}` | Nome do Vendedor | — |
| `{{seller_document}}` | CPF/CNPJ do Vendedor | — |
| `{{seller_email}}` | Email do Vendedor | — |
| `{{seller_phone}}` | Telefone do Vendedor | — |
| `{{seller_address}}` | Endereço do Vendedor — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller}}` | Vendedor (Objeto) — Acesse os campos da parte principal com ponto. | `{{seller.name}} — {{seller.document}}, {{seller.address}}` |
| `{{landlord_name}}` | Nome do Proprietário (Locador) | — |
| `{{landlord_document}}` | CPF/CNPJ do Proprietário (Locador) | — |
| `{{landlord_email}}` | Email do Proprietário (Locador) | — |
| `{{landlord_phone}}` | Telefone do Proprietário (Locador) | — |
| `{{landlord_address}}` | Endereço do Proprietário (Locador) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord}}` | Locador (Objeto) — Acesse os campos da parte principal com ponto. | `{{landlord.name}} — {{landlord.document}}, {{landlord.address}}` |
| `{{owners}}` | Proprietários (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#owners}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/owners}}` |
| `{{guarantors}}` | Fiadores/Avalistas (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#guarantors}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/guarantors}}` |
| `{{witnesses}}` | Testemunhas (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#witnesses}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/witnesses}}` |
| `{{parties}}` | Partes (Lista) — Inclui todas as partes cadastradas e suas qualificações. | `{{#parties}}{{name}} — {{role}} — {{document}}{{/parties}}` |
| `{{sellers}}` | Vendedor (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#sellers}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/sellers}}` |
| `{{landlords}}` | Locador (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#landlords}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/landlords}}` |
| `{{spouses}}` | Cônjuge (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#spouses}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/spouses}}` |
| `{{attorneys}}` | Procurador (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#attorneys}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/attorneys}}` |
| `{{legal_representatives}}` | Representante legal (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#legal_representatives}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/legal_representatives}}` |
| `{{guardians}}` | Tutor (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#guardians}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/guardians}}` |
| `{{curators}}` | Curador (Lista) — Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios. | `{{#curators}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/curators}}` |
| `{{owner}}` | Proprietário (Objeto) — Acesse os campos da parte principal com ponto. | `{{owner.name}} — {{owner.document}}, {{owner.address}}` |
| `{{guarantor}}` | Fiador (Objeto) — Acesse os campos da parte principal com ponto. | `{{guarantor.name}} — {{guarantor.document}}, {{guarantor.address}}` |
| `{{witness}}` | Testemunha (Objeto) — Acesse os campos da parte principal com ponto. | `{{witness.name}} — {{witness.document}}, {{witness.address}}` |
| `{{spouse}}` | Cônjuge (Objeto) — Acesse os campos da parte principal com ponto. | `{{spouse.name}} — {{spouse.document}}, {{spouse.address}}` |
| `{{attorney}}` | Procurador (Objeto) — Acesse os campos da parte principal com ponto. | `{{attorney.name}} — {{attorney.document}}, {{attorney.address}}` |
| `{{legal_representative}}` | Representante legal (Objeto) — Acesse os campos da parte principal com ponto. | `{{legal_representative.name}} — {{legal_representative.document}}, {{legal_representative.address}}` |
| `{{guardian}}` | Tutor (Objeto) — Acesse os campos da parte principal com ponto. | `{{guardian.name}} — {{guardian.document}}, {{guardian.address}}` |
| `{{curator}}` | Curador (Objeto) — Acesse os campos da parte principal com ponto. | `{{curator.name}} — {{curator.document}}, {{curator.address}}` |

### Partes (568)

| Variável | O que entrega | Exemplo |
|---|---|---|
| `{{tenants_count}}` | Inquilinos (Qtd) | — |
| `{{tenants_primary_name}}` | Inquilino primário (Nome) | — |
| `{{tenants_names_list}}` | Inquilinos (Nomes - Lista) | — |
| `{{tenants_documents_list}}` | Inquilinos (Documentos - Lista) | — |
| `{{tenant_2_name}}` | Inquilino 2 (Nome) | — |
| `{{tenant_3_name}}` | Inquilino 3 (Nome) | — |
| `{{tenant_4_name}}` | Inquilino 4 (Nome) | — |
| `{{tenant_5_name}}` | Inquilino 5 (Nome) | — |
| `{{tenant_2_document}}` | Inquilino 2 (Documento) | — |
| `{{tenant_3_document}}` | Inquilino 3 (Documento) | — |
| `{{tenant_4_document}}` | Inquilino 4 (Documento) | — |
| `{{tenant_5_document}}` | Inquilino 5 (Documento) | — |
| `{{tenant_2_email}}` | Inquilino 2 (Email) | — |
| `{{tenant_3_email}}` | Inquilino 3 (Email) | — |
| `{{tenant_4_email}}` | Inquilino 4 (Email) | — |
| `{{tenant_5_email}}` | Inquilino 5 (Email) | — |
| `{{tenant_2_phone}}` | Inquilino 2 (Telefone) | — |
| `{{tenant_3_phone}}` | Inquilino 3 (Telefone) | — |
| `{{tenant_4_phone}}` | Inquilino 4 (Telefone) | — |
| `{{tenant_5_phone}}` | Inquilino 5 (Telefone) | — |
| `{{buyers_count}}` | Compradores (Qtd) | — |
| `{{buyers_primary_name}}` | Comprador primário (Nome) | — |
| `{{buyers_names_list}}` | Compradores (Nomes - Lista) | — |
| `{{buyers_documents_list}}` | Compradores (Documentos - Lista) | — |
| `{{buyer_2_name}}` | Comprador 2 (Nome) | — |
| `{{buyer_3_name}}` | Comprador 3 (Nome) | — |
| `{{buyer_4_name}}` | Comprador 4 (Nome) | — |
| `{{buyer_5_name}}` | Comprador 5 (Nome) | — |
| `{{buyer_2_document}}` | Comprador 2 (Documento) | — |
| `{{buyer_3_document}}` | Comprador 3 (Documento) | — |
| `{{buyer_4_document}}` | Comprador 4 (Documento) | — |
| `{{buyer_5_document}}` | Comprador 5 (Documento) | — |
| `{{buyer_2_email}}` | Comprador 2 (Email) | — |
| `{{buyer_3_email}}` | Comprador 3 (Email) | — |
| `{{buyer_4_email}}` | Comprador 4 (Email) | — |
| `{{buyer_5_email}}` | Comprador 5 (Email) | — |
| `{{buyer_2_phone}}` | Comprador 2 (Telefone) | — |
| `{{buyer_3_phone}}` | Comprador 3 (Telefone) | — |
| `{{buyer_4_phone}}` | Comprador 4 (Telefone) | — |
| `{{buyer_5_phone}}` | Comprador 5 (Telefone) | — |
| `{{owners_count}}` | Proprietários (Qtd) | — |
| `{{owners_primary_name}}` | Proprietário primário (Nome) | — |
| `{{owners_names_list}}` | Proprietários (Nomes - Lista) | — |
| `{{owners_documents_list}}` | Proprietários (Documentos - Lista) | — |
| `{{owner_name}}` | Proprietário 1 (Nome) | — |
| `{{owner_2_name}}` | Proprietário 2 (Nome) | — |
| `{{owner_3_name}}` | Proprietário 3 (Nome) | — |
| `{{owner_4_name}}` | Proprietário 4 (Nome) | — |
| `{{owner_5_name}}` | Proprietário 5 (Nome) | — |
| `{{owner_document}}` | Proprietário 1 (Documento) | — |
| `{{owner_2_document}}` | Proprietário 2 (Documento) | — |
| `{{owner_3_document}}` | Proprietário 3 (Documento) | — |
| `{{owner_4_document}}` | Proprietário 4 (Documento) | — |
| `{{owner_5_document}}` | Proprietário 5 (Documento) | — |
| `{{owner_email}}` | Proprietário 1 (Email) | — |
| `{{owner_2_email}}` | Proprietário 2 (Email) | — |
| `{{owner_3_email}}` | Proprietário 3 (Email) | — |
| `{{owner_4_email}}` | Proprietário 4 (Email) | — |
| `{{owner_5_email}}` | Proprietário 5 (Email) | — |
| `{{owner_phone}}` | Proprietário 1 (Telefone) | — |
| `{{owner_2_phone}}` | Proprietário 2 (Telefone) | — |
| `{{owner_3_phone}}` | Proprietário 3 (Telefone) | — |
| `{{owner_4_phone}}` | Proprietário 4 (Telefone) | — |
| `{{owner_5_phone}}` | Proprietário 5 (Telefone) | — |
| `{{guarantors_count}}` | Fiadores/Avalistas (Qtd) | — |
| `{{guarantors_primary_name}}` | Fiador primário (Nome) | — |
| `{{guarantors_names_list}}` | Fiadores/Avalistas (Nomes - Lista) | — |
| `{{guarantors_documents_list}}` | Fiadores/Avalistas (Documentos - Lista) | — |
| `{{guarantor_name}}` | Fiador 1 (Nome) | — |
| `{{guarantor_2_name}}` | Fiador 2 (Nome) | — |
| `{{guarantor_3_name}}` | Fiador 3 (Nome) | — |
| `{{guarantor_4_name}}` | Fiador 4 (Nome) | — |
| `{{guarantor_5_name}}` | Fiador 5 (Nome) | — |
| `{{guarantor_document}}` | Fiador 1 (Documento) | — |
| `{{guarantor_2_document}}` | Fiador 2 (Documento) | — |
| `{{guarantor_3_document}}` | Fiador 3 (Documento) | — |
| `{{guarantor_4_document}}` | Fiador 4 (Documento) | — |
| `{{guarantor_5_document}}` | Fiador 5 (Documento) | — |
| `{{guarantor_email}}` | Fiador 1 (Email) | — |
| `{{guarantor_2_email}}` | Fiador 2 (Email) | — |
| `{{guarantor_3_email}}` | Fiador 3 (Email) | — |
| `{{guarantor_4_email}}` | Fiador 4 (Email) | — |
| `{{guarantor_5_email}}` | Fiador 5 (Email) | — |
| `{{guarantor_phone}}` | Fiador 1 (Telefone) | — |
| `{{guarantor_2_phone}}` | Fiador 2 (Telefone) | — |
| `{{guarantor_3_phone}}` | Fiador 3 (Telefone) | — |
| `{{guarantor_4_phone}}` | Fiador 4 (Telefone) | — |
| `{{guarantor_5_phone}}` | Fiador 5 (Telefone) | — |
| `{{buyer_birth_date}}` | Comprador: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_birth_date_iso}}` | Comprador: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_identity_document}}` | Comprador: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_nationality}}` | Comprador: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_marital_status}}` | Comprador: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_occupation}}` | Comprador: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_address_street}}` | Comprador: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_address_number}}` | Comprador: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_address_complement}}` | Comprador: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_address_neighborhood}}` | Comprador: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_address_city}}` | Comprador: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_address_state}}` | Comprador: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_address_postal_code}}` | Comprador: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_company_legal_name}}` | Comprador: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_company_trade_name}}` | Comprador: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_company_state_registration}}` | Comprador: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_spouse_name}}` | Comprador: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_spouse_document}}` | Comprador: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_spouse_email}}` | Comprador: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_representative_name}}` | Comprador: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_representative_document}}` | Comprador: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_representative_email}}` | Comprador: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_representation_type}}` | Comprador: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{buyer_represented_party_names_list}}` | Comprador: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_birth_date}}` | Inquilino: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_birth_date_iso}}` | Inquilino: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_identity_document}}` | Inquilino: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_nationality}}` | Inquilino: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_marital_status}}` | Inquilino: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_occupation}}` | Inquilino: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_address_street}}` | Inquilino: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_address_number}}` | Inquilino: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_address_complement}}` | Inquilino: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_address_neighborhood}}` | Inquilino: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_address_city}}` | Inquilino: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_address_state}}` | Inquilino: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_address_postal_code}}` | Inquilino: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_company_legal_name}}` | Inquilino: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_company_trade_name}}` | Inquilino: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_company_state_registration}}` | Inquilino: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_spouse_name}}` | Inquilino: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_spouse_document}}` | Inquilino: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_spouse_email}}` | Inquilino: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_representative_name}}` | Inquilino: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_representative_document}}` | Inquilino: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_representative_email}}` | Inquilino: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_representation_type}}` | Inquilino: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{tenant_represented_party_names_list}}` | Inquilino: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_birth_date}}` | Proprietário: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_birth_date_iso}}` | Proprietário: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_identity_document}}` | Proprietário: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_nationality}}` | Proprietário: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_marital_status}}` | Proprietário: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_occupation}}` | Proprietário: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_address}}` | Proprietário: Endereço completo — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_address_street}}` | Proprietário: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_address_number}}` | Proprietário: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_address_complement}}` | Proprietário: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_address_neighborhood}}` | Proprietário: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_address_city}}` | Proprietário: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_address_state}}` | Proprietário: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_address_postal_code}}` | Proprietário: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_company_legal_name}}` | Proprietário: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_company_trade_name}}` | Proprietário: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_company_state_registration}}` | Proprietário: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_spouse_name}}` | Proprietário: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_spouse_document}}` | Proprietário: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_spouse_email}}` | Proprietário: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_representative_name}}` | Proprietário: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_representative_document}}` | Proprietário: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_representative_email}}` | Proprietário: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_representation_type}}` | Proprietário: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{owner_represented_party_names_list}}` | Proprietário: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{sellers_count}}` | Vendedor (count) | — |
| `{{sellers_primary_name}}` | Vendedor (primary_name) | — |
| `{{sellers_names_list}}` | Vendedor (names_list) | — |
| `{{sellers_documents_list}}` | Vendedor (documents_list) | — |
| `{{seller_birth_date}}` | Vendedor: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_birth_date_iso}}` | Vendedor: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_identity_document}}` | Vendedor: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_nationality}}` | Vendedor: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_marital_status}}` | Vendedor: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_occupation}}` | Vendedor: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_address_street}}` | Vendedor: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_address_number}}` | Vendedor: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_address_complement}}` | Vendedor: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_address_neighborhood}}` | Vendedor: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_address_city}}` | Vendedor: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_address_state}}` | Vendedor: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_address_postal_code}}` | Vendedor: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_company_legal_name}}` | Vendedor: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_company_trade_name}}` | Vendedor: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_company_state_registration}}` | Vendedor: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_spouse_name}}` | Vendedor: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_spouse_document}}` | Vendedor: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_spouse_email}}` | Vendedor: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_representative_name}}` | Vendedor: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_representative_document}}` | Vendedor: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_representative_email}}` | Vendedor: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_representation_type}}` | Vendedor: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_represented_party_names_list}}` | Vendedor: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{seller_2_name}}` | Vendedor 2: Nome | — |
| `{{seller_2_document}}` | Vendedor 2: CPF/CNPJ | — |
| `{{seller_2_email}}` | Vendedor 2: Email | — |
| `{{seller_2_phone}}` | Vendedor 2: Telefone | — |
| `{{seller_3_name}}` | Vendedor 3: Nome | — |
| `{{seller_3_document}}` | Vendedor 3: CPF/CNPJ | — |
| `{{seller_3_email}}` | Vendedor 3: Email | — |
| `{{seller_3_phone}}` | Vendedor 3: Telefone | — |
| `{{seller_4_name}}` | Vendedor 4: Nome | — |
| `{{seller_4_document}}` | Vendedor 4: CPF/CNPJ | — |
| `{{seller_4_email}}` | Vendedor 4: Email | — |
| `{{seller_4_phone}}` | Vendedor 4: Telefone | — |
| `{{seller_5_name}}` | Vendedor 5: Nome | — |
| `{{seller_5_document}}` | Vendedor 5: CPF/CNPJ | — |
| `{{seller_5_email}}` | Vendedor 5: Email | — |
| `{{seller_5_phone}}` | Vendedor 5: Telefone | — |
| `{{landlords_count}}` | Locador (count) | — |
| `{{landlords_primary_name}}` | Locador (primary_name) | — |
| `{{landlords_names_list}}` | Locador (names_list) | — |
| `{{landlords_documents_list}}` | Locador (documents_list) | — |
| `{{landlord_birth_date}}` | Locador: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_birth_date_iso}}` | Locador: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_identity_document}}` | Locador: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_nationality}}` | Locador: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_marital_status}}` | Locador: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_occupation}}` | Locador: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_address_street}}` | Locador: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_address_number}}` | Locador: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_address_complement}}` | Locador: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_address_neighborhood}}` | Locador: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_address_city}}` | Locador: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_address_state}}` | Locador: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_address_postal_code}}` | Locador: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_company_legal_name}}` | Locador: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_company_trade_name}}` | Locador: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_company_state_registration}}` | Locador: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_spouse_name}}` | Locador: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_spouse_document}}` | Locador: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_spouse_email}}` | Locador: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_representative_name}}` | Locador: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_representative_document}}` | Locador: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_representative_email}}` | Locador: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_representation_type}}` | Locador: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_represented_party_names_list}}` | Locador: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{landlord_2_name}}` | Locador 2: Nome | — |
| `{{landlord_2_document}}` | Locador 2: CPF/CNPJ | — |
| `{{landlord_2_email}}` | Locador 2: Email | — |
| `{{landlord_2_phone}}` | Locador 2: Telefone | — |
| `{{landlord_3_name}}` | Locador 3: Nome | — |
| `{{landlord_3_document}}` | Locador 3: CPF/CNPJ | — |
| `{{landlord_3_email}}` | Locador 3: Email | — |
| `{{landlord_3_phone}}` | Locador 3: Telefone | — |
| `{{landlord_4_name}}` | Locador 4: Nome | — |
| `{{landlord_4_document}}` | Locador 4: CPF/CNPJ | — |
| `{{landlord_4_email}}` | Locador 4: Email | — |
| `{{landlord_4_phone}}` | Locador 4: Telefone | — |
| `{{landlord_5_name}}` | Locador 5: Nome | — |
| `{{landlord_5_document}}` | Locador 5: CPF/CNPJ | — |
| `{{landlord_5_email}}` | Locador 5: Email | — |
| `{{landlord_5_phone}}` | Locador 5: Telefone | — |
| `{{guarantor_birth_date}}` | Fiador: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_birth_date_iso}}` | Fiador: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_identity_document}}` | Fiador: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_nationality}}` | Fiador: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_marital_status}}` | Fiador: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_occupation}}` | Fiador: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_address}}` | Fiador: Endereço completo — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_address_street}}` | Fiador: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_address_number}}` | Fiador: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_address_complement}}` | Fiador: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_address_neighborhood}}` | Fiador: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_address_city}}` | Fiador: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_address_state}}` | Fiador: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_address_postal_code}}` | Fiador: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_company_legal_name}}` | Fiador: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_company_trade_name}}` | Fiador: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_company_state_registration}}` | Fiador: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_spouse_name}}` | Fiador: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_spouse_document}}` | Fiador: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_spouse_email}}` | Fiador: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_representative_name}}` | Fiador: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_representative_document}}` | Fiador: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_representative_email}}` | Fiador: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_representation_type}}` | Fiador: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guarantor_represented_party_names_list}}` | Fiador: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witnesses_count}}` | Testemunha (count) | — |
| `{{witnesses_primary_name}}` | Testemunha (primary_name) | — |
| `{{witnesses_names_list}}` | Testemunha (names_list) | — |
| `{{witnesses_documents_list}}` | Testemunha (documents_list) | — |
| `{{witness_birth_date}}` | Testemunha: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_birth_date_iso}}` | Testemunha: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_identity_document}}` | Testemunha: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_nationality}}` | Testemunha: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_marital_status}}` | Testemunha: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_occupation}}` | Testemunha: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_address}}` | Testemunha: Endereço completo — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_address_street}}` | Testemunha: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_address_number}}` | Testemunha: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_address_complement}}` | Testemunha: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_address_neighborhood}}` | Testemunha: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_address_city}}` | Testemunha: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_address_state}}` | Testemunha: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_address_postal_code}}` | Testemunha: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_company_legal_name}}` | Testemunha: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_company_trade_name}}` | Testemunha: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_company_state_registration}}` | Testemunha: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_spouse_name}}` | Testemunha: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_spouse_document}}` | Testemunha: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_spouse_email}}` | Testemunha: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_representative_name}}` | Testemunha: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_representative_document}}` | Testemunha: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_representative_email}}` | Testemunha: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_representation_type}}` | Testemunha: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_represented_party_names_list}}` | Testemunha: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{witness_name}}` | Testemunha 1: Nome | — |
| `{{witness_document}}` | Testemunha 1: CPF/CNPJ | — |
| `{{witness_email}}` | Testemunha 1: Email | — |
| `{{witness_phone}}` | Testemunha 1: Telefone | — |
| `{{witness_2_name}}` | Testemunha 2: Nome | — |
| `{{witness_2_document}}` | Testemunha 2: CPF/CNPJ | — |
| `{{witness_2_email}}` | Testemunha 2: Email | — |
| `{{witness_2_phone}}` | Testemunha 2: Telefone | — |
| `{{witness_3_name}}` | Testemunha 3: Nome | — |
| `{{witness_3_document}}` | Testemunha 3: CPF/CNPJ | — |
| `{{witness_3_email}}` | Testemunha 3: Email | — |
| `{{witness_3_phone}}` | Testemunha 3: Telefone | — |
| `{{witness_4_name}}` | Testemunha 4: Nome | — |
| `{{witness_4_document}}` | Testemunha 4: CPF/CNPJ | — |
| `{{witness_4_email}}` | Testemunha 4: Email | — |
| `{{witness_4_phone}}` | Testemunha 4: Telefone | — |
| `{{witness_5_name}}` | Testemunha 5: Nome | — |
| `{{witness_5_document}}` | Testemunha 5: CPF/CNPJ | — |
| `{{witness_5_email}}` | Testemunha 5: Email | — |
| `{{witness_5_phone}}` | Testemunha 5: Telefone | — |
| `{{spouses_count}}` | Cônjuge (count) | — |
| `{{spouses_primary_name}}` | Cônjuge (primary_name) | — |
| `{{spouses_names_list}}` | Cônjuge (names_list) | — |
| `{{spouses_documents_list}}` | Cônjuge (documents_list) | — |
| `{{spouse_birth_date}}` | Cônjuge: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_birth_date_iso}}` | Cônjuge: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_identity_document}}` | Cônjuge: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_nationality}}` | Cônjuge: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_marital_status}}` | Cônjuge: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_occupation}}` | Cônjuge: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_address}}` | Cônjuge: Endereço completo — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_address_street}}` | Cônjuge: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_address_number}}` | Cônjuge: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_address_complement}}` | Cônjuge: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_address_neighborhood}}` | Cônjuge: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_address_city}}` | Cônjuge: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_address_state}}` | Cônjuge: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_address_postal_code}}` | Cônjuge: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_company_legal_name}}` | Cônjuge: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_company_trade_name}}` | Cônjuge: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_company_state_registration}}` | Cônjuge: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_spouse_name}}` | Cônjuge: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_spouse_document}}` | Cônjuge: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_spouse_email}}` | Cônjuge: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_representative_name}}` | Cônjuge: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_representative_document}}` | Cônjuge: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_representative_email}}` | Cônjuge: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_representation_type}}` | Cônjuge: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_represented_party_names_list}}` | Cônjuge: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{spouse_name}}` | Cônjuge 1: Nome | — |
| `{{spouse_document}}` | Cônjuge 1: CPF/CNPJ | — |
| `{{spouse_email}}` | Cônjuge 1: Email | — |
| `{{spouse_phone}}` | Cônjuge 1: Telefone | — |
| `{{spouse_2_name}}` | Cônjuge 2: Nome | — |
| `{{spouse_2_document}}` | Cônjuge 2: CPF/CNPJ | — |
| `{{spouse_2_email}}` | Cônjuge 2: Email | — |
| `{{spouse_2_phone}}` | Cônjuge 2: Telefone | — |
| `{{spouse_3_name}}` | Cônjuge 3: Nome | — |
| `{{spouse_3_document}}` | Cônjuge 3: CPF/CNPJ | — |
| `{{spouse_3_email}}` | Cônjuge 3: Email | — |
| `{{spouse_3_phone}}` | Cônjuge 3: Telefone | — |
| `{{spouse_4_name}}` | Cônjuge 4: Nome | — |
| `{{spouse_4_document}}` | Cônjuge 4: CPF/CNPJ | — |
| `{{spouse_4_email}}` | Cônjuge 4: Email | — |
| `{{spouse_4_phone}}` | Cônjuge 4: Telefone | — |
| `{{spouse_5_name}}` | Cônjuge 5: Nome | — |
| `{{spouse_5_document}}` | Cônjuge 5: CPF/CNPJ | — |
| `{{spouse_5_email}}` | Cônjuge 5: Email | — |
| `{{spouse_5_phone}}` | Cônjuge 5: Telefone | — |
| `{{attorneys_count}}` | Procurador (count) | — |
| `{{attorneys_primary_name}}` | Procurador (primary_name) | — |
| `{{attorneys_names_list}}` | Procurador (names_list) | — |
| `{{attorneys_documents_list}}` | Procurador (documents_list) | — |
| `{{attorney_birth_date}}` | Procurador: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_birth_date_iso}}` | Procurador: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_identity_document}}` | Procurador: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_nationality}}` | Procurador: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_marital_status}}` | Procurador: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_occupation}}` | Procurador: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_address}}` | Procurador: Endereço completo — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_address_street}}` | Procurador: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_address_number}}` | Procurador: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_address_complement}}` | Procurador: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_address_neighborhood}}` | Procurador: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_address_city}}` | Procurador: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_address_state}}` | Procurador: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_address_postal_code}}` | Procurador: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_company_legal_name}}` | Procurador: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_company_trade_name}}` | Procurador: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_company_state_registration}}` | Procurador: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_spouse_name}}` | Procurador: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_spouse_document}}` | Procurador: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_spouse_email}}` | Procurador: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_representative_name}}` | Procurador: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_representative_document}}` | Procurador: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_representative_email}}` | Procurador: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_representation_type}}` | Procurador: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_represented_party_names_list}}` | Procurador: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{attorney_name}}` | Procurador 1: Nome | — |
| `{{attorney_document}}` | Procurador 1: CPF/CNPJ | — |
| `{{attorney_email}}` | Procurador 1: Email | — |
| `{{attorney_phone}}` | Procurador 1: Telefone | — |
| `{{attorney_2_name}}` | Procurador 2: Nome | — |
| `{{attorney_2_document}}` | Procurador 2: CPF/CNPJ | — |
| `{{attorney_2_email}}` | Procurador 2: Email | — |
| `{{attorney_2_phone}}` | Procurador 2: Telefone | — |
| `{{attorney_3_name}}` | Procurador 3: Nome | — |
| `{{attorney_3_document}}` | Procurador 3: CPF/CNPJ | — |
| `{{attorney_3_email}}` | Procurador 3: Email | — |
| `{{attorney_3_phone}}` | Procurador 3: Telefone | — |
| `{{attorney_4_name}}` | Procurador 4: Nome | — |
| `{{attorney_4_document}}` | Procurador 4: CPF/CNPJ | — |
| `{{attorney_4_email}}` | Procurador 4: Email | — |
| `{{attorney_4_phone}}` | Procurador 4: Telefone | — |
| `{{attorney_5_name}}` | Procurador 5: Nome | — |
| `{{attorney_5_document}}` | Procurador 5: CPF/CNPJ | — |
| `{{attorney_5_email}}` | Procurador 5: Email | — |
| `{{attorney_5_phone}}` | Procurador 5: Telefone | — |
| `{{legal_representatives_count}}` | Representante legal (count) | — |
| `{{legal_representatives_primary_name}}` | Representante legal (primary_name) | — |
| `{{legal_representatives_names_list}}` | Representante legal (names_list) | — |
| `{{legal_representatives_documents_list}}` | Representante legal (documents_list) | — |
| `{{legal_representative_birth_date}}` | Representante legal: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_birth_date_iso}}` | Representante legal: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_identity_document}}` | Representante legal: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_nationality}}` | Representante legal: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_marital_status}}` | Representante legal: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_occupation}}` | Representante legal: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_address}}` | Representante legal: Endereço completo — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_address_street}}` | Representante legal: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_address_number}}` | Representante legal: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_address_complement}}` | Representante legal: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_address_neighborhood}}` | Representante legal: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_address_city}}` | Representante legal: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_address_state}}` | Representante legal: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_address_postal_code}}` | Representante legal: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_company_legal_name}}` | Representante legal: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_company_trade_name}}` | Representante legal: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_company_state_registration}}` | Representante legal: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_spouse_name}}` | Representante legal: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_spouse_document}}` | Representante legal: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_spouse_email}}` | Representante legal: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_representative_name}}` | Representante legal: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_representative_document}}` | Representante legal: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_representative_email}}` | Representante legal: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_representation_type}}` | Representante legal: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_represented_party_names_list}}` | Representante legal: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{legal_representative_name}}` | Representante legal 1: Nome | — |
| `{{legal_representative_document}}` | Representante legal 1: CPF/CNPJ | — |
| `{{legal_representative_email}}` | Representante legal 1: Email | — |
| `{{legal_representative_phone}}` | Representante legal 1: Telefone | — |
| `{{legal_representative_2_name}}` | Representante legal 2: Nome | — |
| `{{legal_representative_2_document}}` | Representante legal 2: CPF/CNPJ | — |
| `{{legal_representative_2_email}}` | Representante legal 2: Email | — |
| `{{legal_representative_2_phone}}` | Representante legal 2: Telefone | — |
| `{{legal_representative_3_name}}` | Representante legal 3: Nome | — |
| `{{legal_representative_3_document}}` | Representante legal 3: CPF/CNPJ | — |
| `{{legal_representative_3_email}}` | Representante legal 3: Email | — |
| `{{legal_representative_3_phone}}` | Representante legal 3: Telefone | — |
| `{{legal_representative_4_name}}` | Representante legal 4: Nome | — |
| `{{legal_representative_4_document}}` | Representante legal 4: CPF/CNPJ | — |
| `{{legal_representative_4_email}}` | Representante legal 4: Email | — |
| `{{legal_representative_4_phone}}` | Representante legal 4: Telefone | — |
| `{{legal_representative_5_name}}` | Representante legal 5: Nome | — |
| `{{legal_representative_5_document}}` | Representante legal 5: CPF/CNPJ | — |
| `{{legal_representative_5_email}}` | Representante legal 5: Email | — |
| `{{legal_representative_5_phone}}` | Representante legal 5: Telefone | — |
| `{{guardians_count}}` | Tutor (count) | — |
| `{{guardians_primary_name}}` | Tutor (primary_name) | — |
| `{{guardians_names_list}}` | Tutor (names_list) | — |
| `{{guardians_documents_list}}` | Tutor (documents_list) | — |
| `{{guardian_birth_date}}` | Tutor: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_birth_date_iso}}` | Tutor: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_identity_document}}` | Tutor: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_nationality}}` | Tutor: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_marital_status}}` | Tutor: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_occupation}}` | Tutor: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_address}}` | Tutor: Endereço completo — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_address_street}}` | Tutor: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_address_number}}` | Tutor: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_address_complement}}` | Tutor: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_address_neighborhood}}` | Tutor: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_address_city}}` | Tutor: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_address_state}}` | Tutor: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_address_postal_code}}` | Tutor: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_company_legal_name}}` | Tutor: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_company_trade_name}}` | Tutor: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_company_state_registration}}` | Tutor: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_spouse_name}}` | Tutor: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_spouse_document}}` | Tutor: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_spouse_email}}` | Tutor: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_representative_name}}` | Tutor: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_representative_document}}` | Tutor: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_representative_email}}` | Tutor: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_representation_type}}` | Tutor: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_represented_party_names_list}}` | Tutor: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{guardian_name}}` | Tutor 1: Nome | — |
| `{{guardian_document}}` | Tutor 1: CPF/CNPJ | — |
| `{{guardian_email}}` | Tutor 1: Email | — |
| `{{guardian_phone}}` | Tutor 1: Telefone | — |
| `{{guardian_2_name}}` | Tutor 2: Nome | — |
| `{{guardian_2_document}}` | Tutor 2: CPF/CNPJ | — |
| `{{guardian_2_email}}` | Tutor 2: Email | — |
| `{{guardian_2_phone}}` | Tutor 2: Telefone | — |
| `{{guardian_3_name}}` | Tutor 3: Nome | — |
| `{{guardian_3_document}}` | Tutor 3: CPF/CNPJ | — |
| `{{guardian_3_email}}` | Tutor 3: Email | — |
| `{{guardian_3_phone}}` | Tutor 3: Telefone | — |
| `{{guardian_4_name}}` | Tutor 4: Nome | — |
| `{{guardian_4_document}}` | Tutor 4: CPF/CNPJ | — |
| `{{guardian_4_email}}` | Tutor 4: Email | — |
| `{{guardian_4_phone}}` | Tutor 4: Telefone | — |
| `{{guardian_5_name}}` | Tutor 5: Nome | — |
| `{{guardian_5_document}}` | Tutor 5: CPF/CNPJ | — |
| `{{guardian_5_email}}` | Tutor 5: Email | — |
| `{{guardian_5_phone}}` | Tutor 5: Telefone | — |
| `{{curators_count}}` | Curador (count) | — |
| `{{curators_primary_name}}` | Curador (primary_name) | — |
| `{{curators_names_list}}` | Curador (names_list) | — |
| `{{curators_documents_list}}` | Curador (documents_list) | — |
| `{{curator_birth_date}}` | Curador: Data de nascimento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_birth_date_iso}}` | Curador: Nascimento (ISO) — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_identity_document}}` | Curador: RG — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_nationality}}` | Curador: Nacionalidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_marital_status}}` | Curador: Estado civil — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_occupation}}` | Curador: Profissão — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_address}}` | Curador: Endereço completo — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_address_street}}` | Curador: Logradouro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_address_number}}` | Curador: Número — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_address_complement}}` | Curador: Complemento — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_address_neighborhood}}` | Curador: Bairro — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_address_city}}` | Curador: Cidade — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_address_state}}` | Curador: UF — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_address_postal_code}}` | Curador: CEP — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_company_legal_name}}` | Curador: Razão social — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_company_trade_name}}` | Curador: Nome fantasia — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_company_state_registration}}` | Curador: Inscrição estadual — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_spouse_name}}` | Curador: Nome do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_spouse_document}}` | Curador: Documento do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_spouse_email}}` | Curador: Email do cônjuge — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_representative_name}}` | Curador: Nome do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_representative_document}}` | Curador: Documento do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_representative_email}}` | Curador: Email do representante — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_representation_type}}` | Curador: Tipo de representação — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_represented_party_names_list}}` | Curador: Nomes das partes representadas — Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio. | — |
| `{{curator_name}}` | Curador 1: Nome | — |
| `{{curator_document}}` | Curador 1: CPF/CNPJ | — |
| `{{curator_email}}` | Curador 1: Email | — |
| `{{curator_phone}}` | Curador 1: Telefone | — |
| `{{curator_2_name}}` | Curador 2: Nome | — |
| `{{curator_2_document}}` | Curador 2: CPF/CNPJ | — |
| `{{curator_2_email}}` | Curador 2: Email | — |
| `{{curator_2_phone}}` | Curador 2: Telefone | — |
| `{{curator_3_name}}` | Curador 3: Nome | — |
| `{{curator_3_document}}` | Curador 3: CPF/CNPJ | — |
| `{{curator_3_email}}` | Curador 3: Email | — |
| `{{curator_3_phone}}` | Curador 3: Telefone | — |
| `{{curator_4_name}}` | Curador 4: Nome | — |
| `{{curator_4_document}}` | Curador 4: CPF/CNPJ | — |
| `{{curator_4_email}}` | Curador 4: Email | — |
| `{{curator_4_phone}}` | Curador 4: Telefone | — |
| `{{curator_5_name}}` | Curador 5: Nome | — |
| `{{curator_5_document}}` | Curador 5: CPF/CNPJ | — |
| `{{curator_5_email}}` | Curador 5: Email | — |
| `{{curator_5_phone}}` | Curador 5: Telefone | — |

### Imóvel (42)

| Variável | O que entrega | Exemplo |
|---|---|---|
| `{{property_title}}` | Título do Imóvel | — |
| `{{property_display_id}}` | Código do Imóvel | — |
| `{{property_address}}` | Endereço Completo do Imóvel | — |
| `{{property_street}}` | Rua do Imóvel | — |
| `{{property_number}}` | Número do Imóvel | — |
| `{{property_complement}}` | Complemento do Imóvel | — |
| `{{property_neighborhood}}` | Bairro do Imóvel | — |
| `{{property_city}}` | Cidade do Imóvel | — |
| `{{property_state}}` | Estado do Imóvel | — |
| `{{property_zip}}` | CEP do Imóvel | — |
| `{{property_registration}}` | Matrícula do Imóvel — Matrícula ainda não disponível no cadastro retornado do imóvel. Preencha na revisão do contrato. | — |
| `{{property_area}}` | Área Total | — |
| `{{property_rooms}}` | Número de Quartos | — |
| `{{property_type}}` | Tipo do Imóvel | — |
| `{{property_listing_type}}` | Tipo de Anúncio (Venda/Locação) | — |
| `{{property_sale_price}}` | Preço de Venda do Imóvel | — |
| `{{property_rent_price}}` | Preço de Locação do Imóvel | — |
| `{{property_condominium_fee}}` | Condomínio (Referência) | — |
| `{{property_property_tax}}` | IPTU (Referência) | — |
| `{{property_owners_count}}` | Quantidade de Proprietários | — |
| `{{property_owner_ids}}` | IDs dos Proprietários | — |
| `{{property_owner_names}}` | Nomes dos Proprietários | — |
| `{{property_capturing_brokers_count}}` | Quantidade de Corretores (Captação) | — |
| `{{property_capturing_brokers_summary}}` | Corretores de Captação (Resumo) | — |
| `{{property_id}}` | ID do imóvel | — |
| `{{property_description}}` | Descrição do imóvel | — |
| `{{property_status}}` | Status do imóvel | — |
| `{{property_condition}}` | Condição do imóvel | — |
| `{{property_country}}` | País do imóvel | — |
| `{{property_state_code}}` | UF do imóvel | — |
| `{{property_postal_code}}` | CEP do imóvel | — |
| `{{property_total_area}}` | Área total (m²) | — |
| `{{property_usable_area}}` | Área útil (m²) | — |
| `{{property_land_area}}` | Área do terreno (m²) | — |
| `{{property_built_area}}` | Área construída (m²) | — |
| `{{property_bedrooms}}` | Quartos | — |
| `{{property_bathrooms}}` | Banheiros | — |
| `{{property_suites}}` | Suítes | — |
| `{{property_parking_spaces}}` | Vagas de garagem | — |
| `{{property_amenities_list}}` | Comodidades do imóvel | — |
| `{{property_capture_exclusivity}}` | Exclusividade da captação | — |
| `{{property_intermediation_contract_id}}` | ID do contrato de intermediação | — |

### Negócio (8)

| Variável | O que entrega | Exemplo |
|---|---|---|
| `{{deal_value}}` | Valor da Negociação | — |
| `{{deal_value_written}}` | Valor por Extenso | — |
| `{{down_payment}}` | Valor de Entrada | — |
| `{{installments}}` | Número de Parcelas | — |
| `{{installment_value}}` | Valor da Parcela | — |
| `{{commission_value}}` | Valor da Comissão | — |
| `{{commission_pct}}` | Porcentagem da Comissão | — |
| `{{payment_method}}` | Forma de Pagamento | — |

### Negociação (50)

| Variável | O que entrega | Exemplo |
|---|---|---|
| `{{deal_id}}` | ID da Negociação | — |
| `{{deal_display_id}}` | Código da Negociação | — |
| `{{deal_type}}` | Tipo da Negociação (SALE/RENT) | — |
| `{{deal_type_label}}` | Tipo do Negócio (Venda/Locação) | — |
| `{{deal_status}}` | Status da Negociação | — |
| `{{proposal_value}}` | Valor da Proposta | — |
| `{{proposal_payment_method}}` | Forma de Pagamento (Proposta) | — |
| `{{proposal_installments}}` | Parcelas (Proposta) | — |
| `{{proposal_down_payment}}` | Entrada (Proposta) | — |
| `{{signers_count}}` | Quantidade de Signatários | — |
| `{{signers_list}}` | Signatários (Lista) | — |
| `{{broker_primary_id}}` | Corretor Responsável (ID) | — |
| `{{broker_primary_name}}` | Corretor Responsável (Nome) | — |
| `{{broker_primary_email}}` | Corretor Responsável (Email) | — |
| `{{broker_participations_count}}` | Participações de Corretores (Qtd) | — |
| `{{broker_participations_summary}}` | Participações de Corretores (Resumo) | — |
| `{{broker_captador_name}}` | Corretor Captador (Nome) — Compatibilidade com modelos existentes. Prefira {{broker_listing_name}}. | — |
| `{{broker_visitador_name}}` | Corretor Visitador (Nome) — Compatibilidade com modelos existentes. Prefira {{broker_visiting_name}}. | — |
| `{{broker_fechamento_name}}` | Corretor Fechamento (Nome) — Compatibilidade com modelos existentes. Prefira {{broker_closing_name}}. | — |
| `{{due_diligence_status}}` | Análise (Status) | — |
| `{{due_diligence_score_serasa}}` | Análise (Score Serasa) | — |
| `{{due_diligence_cnd_status}}` | Certidões (Status) | — |
| `{{due_diligence_legal_opinion}}` | Parecer Jurídico | — |
| `{{due_diligence_updated_at}}` | Análise (Atualizada em) | — |
| `{{checklist_total}}` | Checklist (Total) | — |
| `{{checklist_pending}}` | Checklist (Pendentes) | — |
| `{{checklist_approved}}` | Checklist (Aprovados) | — |
| `{{checklist_rejected}}` | Checklist (Rejeitados) | — |
| `{{checklist_pending_required}}` | Checklist (Pendentes Obrigatórios) | — |
| `{{signers}}` | Signatários (Lista) — Repete o trecho para cada signatário do envelope. | `{{#signers}}{{name}} — {{role}} — {{status}}{{/signers}}` |
| `{{deal_property_id}}` | ID do imóvel da negociação | — |
| `{{deal_customer_id}}` | ID do cliente da negociação | — |
| `{{deal_owner_id}}` | ID do proprietário da negociação | — |
| `{{deal_created_at}}` | Data de criação da negociação | — |
| `{{deal_updated_at}}` | Data de atualização da negociação | — |
| `{{proposal_id}}` | ID da proposta | — |
| `{{proposal_status}}` | Status da proposta | — |
| `{{proposal_counter_value}}` | Valor da contraproposta | — |
| `{{proposal_notes}}` | Observações da proposta | — |
| `{{proposal_created_at}}` | Data da proposta | — |
| `{{broker_primary_registration}}` | CRECI do corretor responsável (captação) | — |
| `{{broker_listing_id}}` | Corretor listing: id | — |
| `{{broker_listing_name}}` | Corretor listing: name | — |
| `{{broker_listing_email}}` | Corretor listing: email | — |
| `{{broker_visiting_id}}` | Corretor visiting: id | — |
| `{{broker_visiting_name}}` | Corretor visiting: name | — |
| `{{broker_visiting_email}}` | Corretor visiting: email | — |
| `{{broker_closing_id}}` | Corretor closing: id | — |
| `{{broker_closing_name}}` | Corretor closing: name | — |
| `{{broker_closing_email}}` | Corretor closing: email | — |

### Financeiro (9)

| Variável | O que entrega | Exemplo |
|---|---|---|
| `{{financed_value}}` | Valor Financiado | — |
| `{{financed_value_written}}` | Valor Financiado por Extenso | — |
| `{{rent_monthly_amount}}` | Aluguel Mensal | — |
| `{{rent_monthly_amount_written}}` | Aluguel Mensal por Extenso | — |
| `{{rent_default_due_day}}` | Vencimento Padrão (Dia) | — |
| `{{rent_first_charge_date}}` | Data do Primeiro Aluguel | — |
| `{{rent_first_charge_type}}` | Tipo do Primeiro Aluguel | — |
| `{{rent_first_charge_type_label}}` | Tipo do Primeiro Aluguel (Texto) | — |
| `{{rent_standard_due_date}}` | Próximo Vencimento Padrão (Data) | — |

### Contrato (29)

| Variável | O que entrega | Exemplo |
|---|---|---|
| `{{company_name}}` | Nome da Imobiliária | — |
| `{{company_legal_name}}` | Razão Social da Imobiliária | — |
| `{{company_document}}` | CNPJ da Imobiliária | — |
| `{{company_email}}` | Email da Imobiliária | — |
| `{{company_phone}}` | Telefone da Imobiliária | — |
| `{{company_address}}` | Endereço da Imobiliária | — |
| `{{contract_id}}` | ID do Contrato | — |
| `{{contract_display_id}}` | Código do Contrato | — |
| `{{contract_status}}` | Status do Contrato | — |
| `{{contract_value}}` | Valor do Contrato | — |
| `{{contract_commission_pct}}` | Comissão do Contrato (%) | — |
| `{{contract_adjust_index}}` | Índice de Reajuste | — |
| `{{contract_auto_adjust}}` | Reajuste Automático | — |
| `{{contract_next_adjustment}}` | Próximo Reajuste | — |
| `{{contract_start_date}}` | Data de Início do Contrato | — |
| `{{contract_end_date}}` | Data de Término do Contrato | — |
| `{{contract_start_date_iso}}` | Data Início (ISO) | — |
| `{{contract_end_date_iso}}` | Data Término (ISO) | — |
| `{{contract_term_months}}` | Prazo do Contrato (Meses) | — |
| `{{contract_type}}` | Tipo do contrato | — |
| `{{contract_value_written}}` | Valor do contrato por extenso | — |
| `{{contract_late_fee_pct}}` | Multa por atraso (%) | — |
| `{{contract_monthly_interest_pct}}` | Juros mensais (%) | — |
| `{{contract_security_deposit}}` | Caução | — |
| `{{contract_security_deposit_written}}` | Caução por extenso | — |
| `{{contract_created_at}}` | Data de criação do contrato | — |
| `{{contract_updated_at}}` | Data de atualização do contrato | — |
| `{{contract_signatories_count}}` | Quantidade de signatários do contrato | — |
| `{{contract_signature_provider}}` | Provedor de assinatura | — |

### Data (22)

| Variável | O que entrega | Exemplo |
|---|---|---|
| `{{current_date}}` | Data Atual (DD/MM/AAAA) | — |
| `{{current_day}}` | Dia Atual | — |
| `{{current_month}}` | Mês Atual | — |
| `{{current_year}}` | Ano Atual | — |
| `{{city_date}}` | Cidade, Data | — |
| `{{current_date_iso}}` | current_date (AAAA-MM-DD) | — |
| `{{current_date_written}}` | current_date (por extenso) | — |
| `{{contract_start_date_written}}` | contract_start_date (por extenso) | — |
| `{{contract_end_date_written}}` | contract_end_date (por extenso) | — |
| `{{rent_first_charge_date_iso}}` | rent_first_charge_date (AAAA-MM-DD) | — |
| `{{rent_first_charge_date_written}}` | rent_first_charge_date (por extenso) | — |
| `{{rent_standard_due_date_iso}}` | rent_standard_due_date (AAAA-MM-DD) | — |
| `{{rent_standard_due_date_written}}` | rent_standard_due_date (por extenso) | — |
| `{{contract_next_adjustment_iso}}` | contract_next_adjustment (AAAA-MM-DD) | — |
| `{{contract_next_adjustment_written}}` | contract_next_adjustment (por extenso) | — |
| `{{due_diligence_updated_at_iso}}` | due_diligence_updated_at (AAAA-MM-DD) | — |
| `{{due_diligence_updated_at_written}}` | due_diligence_updated_at (por extenso) | — |
| `{{current_month_name}}` | Nome do mês atual | — |
| `{{current_month_number}}` | Número do mês atual | — |
| `{{current_date_plus_30_days}}` | Data atual + 30 dias — Troque 30 pelo prazo desejado. Aceita days, business_days (exclui apenas sábados e domingos; feriados são contados), months e years; plus ou minus; sufixos _iso ou _written. | `{{current_date_plus_30_days}}` |
| `{{current_date_plus_5_business_days}}` | Data atual + 5 dias úteis — Dias úteis excluem apenas sábados e domingos; feriados são contados. | `{{current_date_plus_5_business_days}}` |
| `{{contract_start_date_plus_12_months}}` | Início do contrato + 12 meses — Calcula a partir da data de início efetivamente preenchida. Sem data base, fica vazio. | `{{contract_start_date_plus_12_months_written}}` |

