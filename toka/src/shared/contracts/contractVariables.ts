import { isContractDateVariable, parseContractDateVariable } from "./contractDateVariables";
import { MAX_PARTIES_PER_ROLE, PARTY_QUALIFICATION_FIELDS } from "../utils/contractParties";

export interface ContractVariableField {
 key: string;
 label: string;
 kind: "text" | "number" | "boolean";
}

export interface ContractVariable {
 key: string;
 label: string;
 category: 'Pessoas' | 'Partes' | 'Imóvel' | 'Negócio' | 'Negociação' | 'Financeiro' | 'Contrato' | 'Data';
 description?: string;
 kind?: "text" | "number" | "boolean" | "object" | "list";
 fields?: ContractVariableField[];
 example?: string;
 legacyAliasFor?: string;
}

export const CONTRACT_VARIABLE_CATEGORIES = [
 'Pessoas',
 'Partes',
 'Imóvel',
 'Negócio',
 'Negociação',
 'Financeiro',
 'Contrato',
 'Data',
] as const satisfies readonly ContractVariable['category'][];

const BASE_CONTRACT_VARIABLES: ContractVariable[] = [
 // Pessoas - Comprador
 { key: 'buyer_name', label: 'Nome do Comprador', category: 'Pessoas' },
 { key: 'buyer_document', label: 'CPF/CNPJ do Comprador', category: 'Pessoas' },
 { key: 'buyer_email', label: 'Email do Comprador', category: 'Pessoas' },
 { key: 'buyer_phone', label: 'Telefone do Comprador', category: 'Pessoas' },
 { key: 'buyer_address', label: 'Endereço do Comprador', category: 'Pessoas' },

 { key: 'buyer', label: 'Comprador (Objeto)', category: 'Pessoas' },
 { key: 'buyers', label: 'Compradores (Lista)', category: 'Pessoas' },

 { key: 'tenant_name', label: 'Nome do Inquilino', category: 'Pessoas' },
 { key: 'tenant_document', label: 'CPF/CNPJ do Inquilino', category: 'Pessoas' },
 { key: 'tenant_email', label: 'Email do Inquilino', category: 'Pessoas' },
 { key: 'tenant_phone', label: 'Telefone do Inquilino', category: 'Pessoas' },
 { key: 'tenant_address', label: 'Endereço do Inquilino', category: 'Pessoas' },
 
 { key: 'tenant', label: 'Inquilino (Objeto)', category: 'Pessoas' },
 { key: 'tenants', label: 'Inquilinos (Lista)', category: 'Pessoas' },

 // Pessoas - Vendedor
 { key: 'seller_name', label: 'Nome do Vendedor', category: 'Pessoas' },
 { key: 'seller_document', label: 'CPF/CNPJ do Vendedor', category: 'Pessoas' },
 { key: 'seller_email', label: 'Email do Vendedor', category: 'Pessoas' },
 { key: 'seller_phone', label: 'Telefone do Vendedor', category: 'Pessoas' },
 { key: 'seller_address', label: 'Endereço do Vendedor', category: 'Pessoas' },

 { key: 'seller', label: 'Vendedor (Objeto)', category: 'Pessoas' },

 { key: 'landlord_name', label: 'Nome do Proprietário (Locador)', category: 'Pessoas' },
 { key: 'landlord_document', label: 'CPF/CNPJ do Proprietário (Locador)', category: 'Pessoas' },
 { key: 'landlord_email', label: 'Email do Proprietário (Locador)', category: 'Pessoas' },
 { key: 'landlord_phone', label: 'Telefone do Proprietário (Locador)', category: 'Pessoas' },
 { key: 'landlord_address', label: 'Endereço do Proprietário (Locador)', category: 'Pessoas' },

 { key: 'landlord', label: 'Locador (Objeto)', category: 'Pessoas' },
 { key: 'owners', label: 'Proprietários (Lista)', category: 'Pessoas' },
 { key: 'guarantors', label: 'Fiadores/Avalistas (Lista)', category: 'Pessoas' },
 { key: 'witnesses', label: 'Testemunhas (Lista)', category: 'Pessoas' },
 { key: 'parties', label: 'Partes (Lista)', category: 'Pessoas' },

 { key: 'tenants_count', label: 'Inquilinos (Qtd)', category: 'Partes' },
 { key: 'tenants_primary_name', label: 'Inquilino primário (Nome)', category: 'Partes' },
 { key: 'tenants_names_list', label: 'Inquilinos (Nomes - Lista)', category: 'Partes' },
 { key: 'tenants_documents_list', label: 'Inquilinos (Documentos - Lista)', category: 'Partes' },
 { key: 'tenant_2_name', label: 'Inquilino 2 (Nome)', category: 'Partes' },
 { key: 'tenant_3_name', label: 'Inquilino 3 (Nome)', category: 'Partes' },
 { key: 'tenant_4_name', label: 'Inquilino 4 (Nome)', category: 'Partes' },
 { key: 'tenant_5_name', label: 'Inquilino 5 (Nome)', category: 'Partes' },
 { key: 'tenant_2_document', label: 'Inquilino 2 (Documento)', category: 'Partes' },
 { key: 'tenant_3_document', label: 'Inquilino 3 (Documento)', category: 'Partes' },
 { key: 'tenant_4_document', label: 'Inquilino 4 (Documento)', category: 'Partes' },
 { key: 'tenant_5_document', label: 'Inquilino 5 (Documento)', category: 'Partes' },
 { key: 'tenant_2_email', label: 'Inquilino 2 (Email)', category: 'Partes' },
 { key: 'tenant_3_email', label: 'Inquilino 3 (Email)', category: 'Partes' },
 { key: 'tenant_4_email', label: 'Inquilino 4 (Email)', category: 'Partes' },
 { key: 'tenant_5_email', label: 'Inquilino 5 (Email)', category: 'Partes' },
 { key: 'tenant_2_phone', label: 'Inquilino 2 (Telefone)', category: 'Partes' },
 { key: 'tenant_3_phone', label: 'Inquilino 3 (Telefone)', category: 'Partes' },
 { key: 'tenant_4_phone', label: 'Inquilino 4 (Telefone)', category: 'Partes' },
 { key: 'tenant_5_phone', label: 'Inquilino 5 (Telefone)', category: 'Partes' },

 { key: 'buyers_count', label: 'Compradores (Qtd)', category: 'Partes' },
 { key: 'buyers_primary_name', label: 'Comprador primário (Nome)', category: 'Partes' },
 { key: 'buyers_names_list', label: 'Compradores (Nomes - Lista)', category: 'Partes' },
 { key: 'buyers_documents_list', label: 'Compradores (Documentos - Lista)', category: 'Partes' },
 { key: 'buyer_2_name', label: 'Comprador 2 (Nome)', category: 'Partes' },
 { key: 'buyer_3_name', label: 'Comprador 3 (Nome)', category: 'Partes' },
 { key: 'buyer_4_name', label: 'Comprador 4 (Nome)', category: 'Partes' },
 { key: 'buyer_5_name', label: 'Comprador 5 (Nome)', category: 'Partes' },
 { key: 'buyer_2_document', label: 'Comprador 2 (Documento)', category: 'Partes' },
 { key: 'buyer_3_document', label: 'Comprador 3 (Documento)', category: 'Partes' },
 { key: 'buyer_4_document', label: 'Comprador 4 (Documento)', category: 'Partes' },
 { key: 'buyer_5_document', label: 'Comprador 5 (Documento)', category: 'Partes' },
 { key: 'buyer_2_email', label: 'Comprador 2 (Email)', category: 'Partes' },
 { key: 'buyer_3_email', label: 'Comprador 3 (Email)', category: 'Partes' },
 { key: 'buyer_4_email', label: 'Comprador 4 (Email)', category: 'Partes' },
 { key: 'buyer_5_email', label: 'Comprador 5 (Email)', category: 'Partes' },
 { key: 'buyer_2_phone', label: 'Comprador 2 (Telefone)', category: 'Partes' },
 { key: 'buyer_3_phone', label: 'Comprador 3 (Telefone)', category: 'Partes' },
 { key: 'buyer_4_phone', label: 'Comprador 4 (Telefone)', category: 'Partes' },
 { key: 'buyer_5_phone', label: 'Comprador 5 (Telefone)', category: 'Partes' },

 { key: 'owners_count', label: 'Proprietários (Qtd)', category: 'Partes' },
 { key: 'owners_primary_name', label: 'Proprietário primário (Nome)', category: 'Partes' },
 { key: 'owners_names_list', label: 'Proprietários (Nomes - Lista)', category: 'Partes' },
 { key: 'owners_documents_list', label: 'Proprietários (Documentos - Lista)', category: 'Partes' },
 { key: 'owner_name', label: 'Proprietário 1 (Nome)', category: 'Partes' },
 { key: 'owner_2_name', label: 'Proprietário 2 (Nome)', category: 'Partes' },
 { key: 'owner_3_name', label: 'Proprietário 3 (Nome)', category: 'Partes' },
 { key: 'owner_4_name', label: 'Proprietário 4 (Nome)', category: 'Partes' },
 { key: 'owner_5_name', label: 'Proprietário 5 (Nome)', category: 'Partes' },
 { key: 'owner_document', label: 'Proprietário 1 (Documento)', category: 'Partes' },
 { key: 'owner_2_document', label: 'Proprietário 2 (Documento)', category: 'Partes' },
 { key: 'owner_3_document', label: 'Proprietário 3 (Documento)', category: 'Partes' },
 { key: 'owner_4_document', label: 'Proprietário 4 (Documento)', category: 'Partes' },
 { key: 'owner_5_document', label: 'Proprietário 5 (Documento)', category: 'Partes' },
 { key: 'owner_email', label: 'Proprietário 1 (Email)', category: 'Partes' },
 { key: 'owner_2_email', label: 'Proprietário 2 (Email)', category: 'Partes' },
 { key: 'owner_3_email', label: 'Proprietário 3 (Email)', category: 'Partes' },
 { key: 'owner_4_email', label: 'Proprietário 4 (Email)', category: 'Partes' },
 { key: 'owner_5_email', label: 'Proprietário 5 (Email)', category: 'Partes' },
 { key: 'owner_phone', label: 'Proprietário 1 (Telefone)', category: 'Partes' },
 { key: 'owner_2_phone', label: 'Proprietário 2 (Telefone)', category: 'Partes' },
 { key: 'owner_3_phone', label: 'Proprietário 3 (Telefone)', category: 'Partes' },
 { key: 'owner_4_phone', label: 'Proprietário 4 (Telefone)', category: 'Partes' },
 { key: 'owner_5_phone', label: 'Proprietário 5 (Telefone)', category: 'Partes' },

 { key: 'guarantors_count', label: 'Fiadores/Avalistas (Qtd)', category: 'Partes' },
 { key: 'guarantors_primary_name', label: 'Fiador primário (Nome)', category: 'Partes' },
 { key: 'guarantors_names_list', label: 'Fiadores/Avalistas (Nomes - Lista)', category: 'Partes' },
 { key: 'guarantors_documents_list', label: 'Fiadores/Avalistas (Documentos - Lista)', category: 'Partes' },
 { key: 'guarantor_name', label: 'Fiador 1 (Nome)', category: 'Partes' },
 { key: 'guarantor_2_name', label: 'Fiador 2 (Nome)', category: 'Partes' },
 { key: 'guarantor_3_name', label: 'Fiador 3 (Nome)', category: 'Partes' },
 { key: 'guarantor_4_name', label: 'Fiador 4 (Nome)', category: 'Partes' },
 { key: 'guarantor_5_name', label: 'Fiador 5 (Nome)', category: 'Partes' },
 { key: 'guarantor_document', label: 'Fiador 1 (Documento)', category: 'Partes' },
 { key: 'guarantor_2_document', label: 'Fiador 2 (Documento)', category: 'Partes' },
 { key: 'guarantor_3_document', label: 'Fiador 3 (Documento)', category: 'Partes' },
 { key: 'guarantor_4_document', label: 'Fiador 4 (Documento)', category: 'Partes' },
 { key: 'guarantor_5_document', label: 'Fiador 5 (Documento)', category: 'Partes' },
 { key: 'guarantor_email', label: 'Fiador 1 (Email)', category: 'Partes' },
 { key: 'guarantor_2_email', label: 'Fiador 2 (Email)', category: 'Partes' },
 { key: 'guarantor_3_email', label: 'Fiador 3 (Email)', category: 'Partes' },
 { key: 'guarantor_4_email', label: 'Fiador 4 (Email)', category: 'Partes' },
 { key: 'guarantor_5_email', label: 'Fiador 5 (Email)', category: 'Partes' },
 { key: 'guarantor_phone', label: 'Fiador 1 (Telefone)', category: 'Partes' },
 { key: 'guarantor_2_phone', label: 'Fiador 2 (Telefone)', category: 'Partes' },
 { key: 'guarantor_3_phone', label: 'Fiador 3 (Telefone)', category: 'Partes' },
 { key: 'guarantor_4_phone', label: 'Fiador 4 (Telefone)', category: 'Partes' },
 { key: 'guarantor_5_phone', label: 'Fiador 5 (Telefone)', category: 'Partes' },

 // Imóvel
 { key: 'property_title', label: 'Título do Imóvel', category: 'Imóvel' },
 { key: 'property_display_id', label: 'Código do Imóvel', category: 'Imóvel' },
 { key: 'property_address', label: 'Endereço Completo do Imóvel', category: 'Imóvel' },
 { key: 'property_street', label: 'Rua do Imóvel', category: 'Imóvel' },
 { key: 'property_number', label: 'Número do Imóvel', category: 'Imóvel' },
 { key: 'property_complement', label: 'Complemento do Imóvel', category: 'Imóvel' },
 { key: 'property_neighborhood', label: 'Bairro do Imóvel', category: 'Imóvel' },
 { key: 'property_city', label: 'Cidade do Imóvel', category: 'Imóvel' },
 { key: 'property_state', label: 'Estado do Imóvel', category: 'Imóvel' },
 { key: 'property_zip', label: 'CEP do Imóvel', category: 'Imóvel' },
 { key: 'property_registration', label: 'Matrícula do Imóvel', category: 'Imóvel' },
 { key: 'property_area', label: 'Área Total', category: 'Imóvel' },
 { key: 'property_rooms', label: 'Número de Quartos', category: 'Imóvel' },
 { key: 'property_type', label: 'Tipo do Imóvel', category: 'Imóvel' },
 { key: 'property_listing_type', label: 'Tipo de Anúncio (Venda/Locação)', category: 'Imóvel' },
 { key: 'property_sale_price', label: 'Preço de Venda do Imóvel', category: 'Imóvel' },
 { key: 'property_rent_price', label: 'Preço de Locação do Imóvel', category: 'Imóvel' },
 { key: 'property_condominium_fee', label: 'Condomínio (Referência)', category: 'Imóvel' },
 { key: 'property_property_tax', label: 'IPTU (Referência)', category: 'Imóvel' },
 { key: 'property_owners_count', label: 'Quantidade de Proprietários', category: 'Imóvel' },
 { key: 'property_owner_ids', label: 'IDs dos Proprietários', category: 'Imóvel' },
 { key: 'property_owner_names', label: 'Nomes dos Proprietários', category: 'Imóvel' },
 { key: 'property_capturing_brokers_count', label: 'Quantidade de Corretores (Captação)', category: 'Imóvel' },
 { key: 'property_capturing_brokers_summary', label: 'Corretores de Captação (Resumo)', category: 'Imóvel' },

 // Negócio
 { key: 'deal_value', label: 'Valor da Negociação', category: 'Negócio' },
 { key: 'deal_value_written', label: 'Valor por Extenso', category: 'Negócio' },
 { key: 'down_payment', label: 'Valor de Entrada', category: 'Negócio' },
 { key: 'installments', label: 'Número de Parcelas', category: 'Negócio' },
 { key: 'installment_value', label: 'Valor da Parcela', category: 'Negócio' },
 { key: 'commission_value', label: 'Valor da Comissão', category: 'Negócio' },
 { key: 'commission_pct', label: 'Porcentagem da Comissão', category: 'Negócio' },
 { key: 'payment_method', label: 'Forma de Pagamento', category: 'Negócio' },

 // Negociação
 { key: 'deal_id', label: 'ID da Negociação', category: 'Negociação' },
 { key: 'deal_display_id', label: 'Código da Negociação', category: 'Negociação' },
 { key: 'deal_type', label: 'Tipo da Negociação (SALE/RENT)', category: 'Negociação' },
 { key: 'deal_type_label', label: 'Tipo do Negócio (Venda/Locação)', category: 'Negociação' },
 { key: 'deal_status', label: 'Status da Negociação', category: 'Negociação' },
 { key: 'proposal_value', label: 'Valor da Proposta', category: 'Negociação' },
 { key: 'proposal_payment_method', label: 'Forma de Pagamento (Proposta)', category: 'Negociação' },
 { key: 'proposal_installments', label: 'Parcelas (Proposta)', category: 'Negociação' },
 { key: 'proposal_down_payment', label: 'Entrada (Proposta)', category: 'Negociação' },
 { key: 'signers_count', label: 'Quantidade de Signatários', category: 'Negociação' },
 { key: 'signers_list', label: 'Signatários (Lista)', category: 'Negociação' },
 { key: 'broker_primary_id', label: 'Corretor Responsável (ID)', category: 'Negociação' },
 { key: 'broker_primary_name', label: 'Corretor Responsável (Nome)', category: 'Negociação' },
 { key: 'broker_primary_email', label: 'Corretor Responsável (Email)', category: 'Negociação' },
 { key: 'broker_participations_count', label: 'Participações de Corretores (Qtd)', category: 'Negociação' },
 { key: 'broker_participations_summary', label: 'Participações de Corretores (Resumo)', category: 'Negociação' },
 { key: 'broker_captador_name', label: 'Corretor Captador (Nome)', category: 'Negociação' },
 { key: 'broker_visitador_name', label: 'Corretor Visitador (Nome)', category: 'Negociação' },
 { key: 'broker_fechamento_name', label: 'Corretor Fechamento (Nome)', category: 'Negociação' },
 { key: 'due_diligence_status', label: 'Análise (Status)', category: 'Negociação' },
 { key: 'due_diligence_score_serasa', label: 'Análise (Score Serasa)', category: 'Negociação' },
 { key: 'due_diligence_cnd_status', label: 'Certidões (Status)', category: 'Negociação' },
 { key: 'due_diligence_legal_opinion', label: 'Parecer Jurídico', category: 'Negociação' },
 { key: 'due_diligence_updated_at', label: 'Análise (Atualizada em)', category: 'Negociação' },
 { key: 'checklist_total', label: 'Checklist (Total)', category: 'Negociação' },
 { key: 'checklist_pending', label: 'Checklist (Pendentes)', category: 'Negociação' },
 { key: 'checklist_approved', label: 'Checklist (Aprovados)', category: 'Negociação' },
 { key: 'checklist_rejected', label: 'Checklist (Rejeitados)', category: 'Negociação' },
 { key: 'checklist_pending_required', label: 'Checklist (Pendentes Obrigatórios)', category: 'Negociação' },

 // Financeiro
 { key: 'financed_value', label: 'Valor Financiado', category: 'Financeiro' },
 { key: 'financed_value_written', label: 'Valor Financiado por Extenso', category: 'Financeiro' },
 { key: 'rent_monthly_amount', label: 'Aluguel Mensal', category: 'Financeiro' },
 { key: 'rent_monthly_amount_written', label: 'Aluguel Mensal por Extenso', category: 'Financeiro' },
 { key: 'rent_default_due_day', label: 'Vencimento Padrão (Dia)', category: 'Financeiro' },
 { key: 'rent_first_charge_date', label: 'Data do Primeiro Aluguel', category: 'Financeiro' },
 { key: 'rent_first_charge_type', label: 'Tipo do Primeiro Aluguel', category: 'Financeiro' },
 { key: 'rent_first_charge_type_label', label: 'Tipo do Primeiro Aluguel (Texto)', category: 'Financeiro' },
 { key: 'rent_standard_due_date', label: 'Próximo Vencimento Padrão (Data)', category: 'Financeiro' },

 // Contrato
 { key: 'company_name', label: 'Nome da Imobiliária', category: 'Contrato' },
 { key: 'company_legal_name', label: 'Razão Social da Imobiliária', category: 'Contrato' },
 { key: 'company_document', label: 'CNPJ da Imobiliária', category: 'Contrato' },
 { key: 'company_email', label: 'Email da Imobiliária', category: 'Contrato' },
 { key: 'company_phone', label: 'Telefone da Imobiliária', category: 'Contrato' },
 { key: 'company_address', label: 'Endereço da Imobiliária', category: 'Contrato' },

 { key: 'contract_id', label: 'ID do Contrato', category: 'Contrato' },
 { key: 'contract_display_id', label: 'Código do Contrato', category: 'Contrato' },
 { key: 'contract_status', label: 'Status do Contrato', category: 'Contrato' },
 { key: 'contract_value', label: 'Valor do Contrato', category: 'Contrato' },
 { key: 'contract_commission_pct', label: 'Comissão do Contrato (%)', category: 'Contrato' },
 { key: 'contract_adjust_index', label: 'Índice de Reajuste', category: 'Contrato' },
 { key: 'contract_auto_adjust', label: 'Reajuste Automático', category: 'Contrato' },
 { key: 'contract_next_adjustment', label: 'Próximo Reajuste', category: 'Contrato' },
 { key: 'contract_start_date', label: 'Data de Início do Contrato', category: 'Contrato' },
 { key: 'contract_end_date', label: 'Data de Término do Contrato', category: 'Contrato' },
 { key: 'contract_start_date_iso', label: 'Data Início (ISO)', category: 'Contrato' },
 { key: 'contract_end_date_iso', label: 'Data Término (ISO)', category: 'Contrato' },
 { key: 'contract_term_months', label: 'Prazo do Contrato (Meses)', category: 'Contrato' },

 // Datas
 { key: 'current_date', label: 'Data Atual (DD/MM/AAAA)', category: 'Data' },
 { key: 'current_day', label: 'Dia Atual', category: 'Data' },
 { key: 'current_month', label: 'Mês Atual', category: 'Data' },
 { key: 'current_year', label: 'Ano Atual', category: 'Data' },
 { key: 'city_date', label: 'Cidade, Data', category: 'Data' },
];

const PARTY_FIELD_LABELS: Record<string, string> = {
 id: "ID da parte", lead_id: "ID do cliente", account_id: "ID da conta", name: "Nome", document: "CPF/CNPJ",
 email: "Email", phone: "Telefone", role: "Papel", kind: "Pessoa física/jurídica", is_primary: "Parte principal",
 share_pct: "Participação (%)", signature_required: "Assinatura obrigatória", birth_date: "Data de nascimento",
 birth_date_iso: "Nascimento (ISO)", identity_document: "RG", nationality: "Nacionalidade", marital_status: "Estado civil",
 occupation: "Profissão", address: "Endereço completo", address_street: "Logradouro", address_number: "Número",
 address_complement: "Complemento", address_neighborhood: "Bairro", address_city: "Cidade", address_state: "UF",
 address_postal_code: "CEP", company_legal_name: "Razão social", company_trade_name: "Nome fantasia",
 company_state_registration: "Inscrição estadual", spouse_name: "Nome do cônjuge", spouse_document: "Documento do cônjuge",
 spouse_email: "Email do cônjuge", spouse_of_party_id: "ID da parte vinculada ao cônjuge", no_spouse_declared: "Sem cônjuge declarado",
 representation_type: "Tipo de representação", representative_party_id: "ID do representante",
 representative_name: "Nome do representante", representative_document: "Documento do representante",
 representative_email: "Email do representante", represented_party_names_list: "Nomes das partes representadas",
 requires_principal_signature: "Exige assinatura do representado", allow_representative_proposal: "Representante pode propor",
};

export const CONTRACT_PARTY_FIELDS: ContractVariableField[] = Object.entries(PARTY_FIELD_LABELS).map(([key, label]) => ({
 key,
 label,
 kind: key === "share_pct" ? "number" : ["is_primary", "signature_required", "no_spouse_declared", "requires_principal_signature", "allow_representative_proposal"].includes(key) ? "boolean" : "text",
}));

const PARTY_ROLES = [
 ["buyer", "buyers", "Comprador"], ["tenant", "tenants", "Inquilino"], ["owner", "owners", "Proprietário"],
 ["seller", "sellers", "Vendedor"], ["landlord", "landlords", "Locador"],
 ["guarantor", "guarantors", "Fiador"], ["witness", "witnesses", "Testemunha"], ["spouse", "spouses", "Cônjuge"],
 ["attorney", "attorneys", "Procurador"], ["legal_representative", "legal_representatives", "Representante legal"],
 ["guardian", "guardians", "Tutor"], ["curator", "curators", "Curador"],
] as const;

const additionalVariables: ContractVariable[] = [
 ...PARTY_ROLES.flatMap(([singular, plural, label]): ContractVariable[] => [
  { key: plural, label: `${label} (Lista)`, category: "Pessoas", kind: "list", fields: CONTRACT_PARTY_FIELDS,
    description: "Repete o trecho para cada parte cadastrada, sem limite de quantidade. Campos opcionais sem informação ficam vazios.",
    example: `{{#${plural}}}{{name}} — {{document}}, {{occupation}}, {{address}}{{/${plural}}}` },
  ...["count", "primary_name", "names_list", "documents_list"].map((field): ContractVariable => ({
   key: `${plural}_${field}`, label: `${label} (${field})`, category: "Partes", kind: field === "count" ? "number" : "text",
  })),
  ...PARTY_QUALIFICATION_FIELDS.map((field): ContractVariable => ({
   key: `${singular}_${field}`, label: `${label}: ${PARTY_FIELD_LABELS[field]}`, category: "Partes",
   description: "Dados da parte principal cadastrada e dos vínculos explícitos de cônjuge/representação. Sem informação, fica vazio.",
  })),
  ...Array.from({ length: MAX_PARTIES_PER_ROLE }, (_, i) => ["name", "document", "email", "phone"].map((field): ContractVariable => ({
   key: `${singular}${i ? `_${i + 1}` : ""}_${field}`, label: `${label} ${i + 1}: ${PARTY_FIELD_LABELS[field]}`, category: "Partes",
  }))).flat(),
 ]),
 ...PARTY_ROLES.map(([singular, , label]): ContractVariable => ({
   key: singular,
   label: `${label} (Objeto)`,
   category: "Pessoas",
   kind: "object",
   fields: CONTRACT_PARTY_FIELDS,
   description: "Acesse os campos da parte principal com ponto.",
   example: `{{${singular}.name}} — {{${singular}.document}}, {{${singular}.address}}`,
 })),
 ...PARTY_ROLES.flatMap(([singular]): ContractVariable[] =>
   PARTY_QUALIFICATION_FIELDS.map((field): ContractVariable => ({
     key: `${singular}_${field}`,
     label: `${singular}: ${PARTY_FIELD_LABELS[field]}`,
     category: "Partes",
   })),
 ),
 { key: "parties", label: "Todas as partes (Lista)", category: "Pessoas", kind: "list", fields: CONTRACT_PARTY_FIELDS,
   description: "Inclui todas as partes cadastradas e suas qualificações.", example: "{{#parties}}{{name}} — {{role}} — {{document}}{{/parties}}" },
 { key: "signers", label: "Signatários (Lista)", category: "Negociação", kind: "list", fields: [
   { key: "name", label: "Nome", kind: "text" }, { key: "email", label: "Email", kind: "text" },
   { key: "role", label: "Papel", kind: "text" }, { key: "status", label: "Status", kind: "text" },
   { key: "signed_at", label: "Assinado em", kind: "text" }, { key: "is_required", label: "Assinatura obrigatória", kind: "text" },
 ], description: "Repete o trecho para cada signatário do envelope.", example: "{{#signers}}{{name}} — {{role}} — {{status}}{{/signers}}" },
 ...[
  ["property_id", "ID do imóvel"], ["property_description", "Descrição do imóvel"], ["property_status", "Status do imóvel"],
  ["property_condition", "Condição do imóvel"], ["property_country", "País do imóvel"], ["property_state_code", "UF do imóvel"],
  ["property_postal_code", "CEP do imóvel"], ["property_total_area", "Área total (m²)"], ["property_usable_area", "Área útil (m²)"],
  ["property_land_area", "Área do terreno (m²)"], ["property_built_area", "Área construída (m²)"],
  ["property_bedrooms", "Quartos"], ["property_bathrooms", "Banheiros"], ["property_suites", "Suítes"],
  ["property_parking_spaces", "Vagas de garagem"], ["property_amenities_list", "Comodidades do imóvel"],
  ["property_capture_exclusivity", "Exclusividade da captação"], ["property_intermediation_contract_id", "ID do contrato de intermediação"],
 ].map(([key, label]): ContractVariable => ({ key, label, category: "Imóvel" })),
 ...[
  ["deal_property_id", "ID do imóvel da negociação"], ["deal_customer_id", "ID do cliente da negociação"],
  ["deal_owner_id", "ID do proprietário da negociação"], ["deal_created_at", "Data de criação da negociação"],
  ["deal_updated_at", "Data de atualização da negociação"], ["proposal_id", "ID da proposta"], ["proposal_status", "Status da proposta"],
  ["proposal_counter_value", "Valor da contraproposta"], ["proposal_notes", "Observações da proposta"], ["proposal_created_at", "Data da proposta"],
  ["broker_primary_registration", "CRECI do corretor responsável (captação)"],
 ].map(([key, label]): ContractVariable => ({ key, label, category: "Negociação" })),
 ...["listing", "visiting", "closing"].flatMap((role) => ["id", "name", "email"].map((field): ContractVariable => ({
  key: `broker_${role}_${field}`, label: `Corretor ${role}: ${field}`, category: "Negociação",
 }))),
 ...[
  ["contract_type", "Tipo do contrato"], ["contract_value_written", "Valor do contrato por extenso"],
  ["contract_late_fee_pct", "Multa por atraso (%)"], ["contract_monthly_interest_pct", "Juros mensais (%)"],
  ["contract_security_deposit", "Caução"], ["contract_security_deposit_written", "Caução por extenso"],
  ["contract_created_at", "Data de criação do contrato"], ["contract_updated_at", "Data de atualização do contrato"],
  ["contract_signatories_count", "Quantidade de signatários do contrato"], ["contract_signature_provider", "Provedor de assinatura"],
 ].map(([key, label]): ContractVariable => ({ key, label, category: "Contrato" })),
 ...["current_date", "contract_start_date", "contract_end_date", "rent_first_charge_date", "rent_standard_due_date", "contract_next_adjustment", "due_diligence_updated_at"].flatMap((base) => ["iso", "written"].map((format): ContractVariable => ({
  key: `${base}_${format}`, label: `${base} (${format === "iso" ? "AAAA-MM-DD" : "por extenso"})`, category: "Data",
 }))),
 { key: "current_month_name", label: "Nome do mês atual", category: "Data" },
 { key: "current_month_number", label: "Número do mês atual", category: "Data" },
 { key: "current_date_plus_30_days", label: "Data atual + 30 dias", category: "Data", description: "Troque 30 pelo prazo desejado. Aceita days, business_days (exclui apenas sábados e domingos; feriados são contados), months e years; plus ou minus; sufixos _iso ou _written.", example: "{{current_date_plus_30_days}}" },
 { key: "current_date_plus_5_business_days", label: "Data atual + 5 dias úteis", category: "Data", description: "Dias úteis excluem apenas sábados e domingos; feriados são contados.", example: "{{current_date_plus_5_business_days}}" },
 { key: "contract_start_date_plus_12_months", label: "Início do contrato + 12 meses", category: "Data", description: "Calcula a partir da data de início efetivamente preenchida. Sem data base, fica vazio.", example: "{{contract_start_date_plus_12_months_written}}" },
];

const variableMap = new Map(BASE_CONTRACT_VARIABLES.map((variable) => [variable.key, variable]));
for (const variable of additionalVariables) {
 const existing = variableMap.get(variable.key);
 variableMap.set(variable.key, existing ? { ...existing, ...variable, label: existing.label, category: existing.category } : variable);
}
for (const [legacy, canonical] of [["captador", "listing"], ["visitador", "visiting"], ["fechamento", "closing"]]) {
 const key = `broker_${legacy}_name`;
 const variable = variableMap.get(key);
 if (variable) variableMap.set(key, { ...variable, legacyAliasFor: `broker_${canonical}_name`, description: `Compatibilidade com modelos existentes. Prefira {{broker_${canonical}_name}}.` });
}
const registration = variableMap.get("property_registration");
if (registration) variableMap.set(registration.key, { ...registration, description: "Matrícula ainda não disponível no cadastro retornado do imóvel. Preencha na revisão do contrato." });

export const CONTRACT_VARIABLES: ContractVariable[] = [...variableMap.values()];

export const getVariableByKey = (key: string): ContractVariable | undefined => {
 const exact = variableMap.get(key);
 if (exact) return exact;
 const [root, fieldKey, ...rest] = key.split(".");
 const parent = variableMap.get(root);
 const indexedListField = parent?.kind === "list" && rest.length === 1
  && /^(0|[1-9]\d*)$/.test(fieldKey) && Number.isSafeInteger(Number(fieldKey));
 const resolvedFieldKey = !rest.length ? fieldKey : indexedListField ? rest[0] : undefined;
 const field = parent?.fields?.find((item) => item.key === resolvedFieldKey);
 if (field && parent) return { key, label: `${parent.label}: ${field.label}`, category: parent.category, kind: field.kind };
 const date = parseContractDateVariable(key);
 if (date) return { key, label: key, category: "Data", description: "Data calculada a partir da data base preenchida. Dias úteis excluem apenas sábados e domingos; feriados são contados.", example: `{{${key}}}` };
 return undefined;
};

export const isSystemVariable = (key: string): boolean => !!getVariableByKey(key) || isContractDateVariable(key);

export const slugifyVariableName = (text: string): string => {
 return text
 .toLowerCase()
 .normalize("NFD")
 .replace(/[\u0300-\u036f]/g, "")
 .replace(/[^a-z0-9\s_]/g, "")
 .trim()
 .replace(/\s+/g, "_")
 .slice(0, 40);
};
