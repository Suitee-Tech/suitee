export type LeadStatus =
 | 'Novo'
 | 'Em Atendimento'
 | 'Qualificado'
 | 'Visita Agendada'
 | 'Em Negociação'
 | 'Proposta Enviada'
 | 'Convertido'
 | 'Perdido';

export type LeadType = 'captacao' | 'compra_venda' | 'locacao';

export type LeadSource = 'Site' | 'Indicacao' | 'Redes Sociais' | 'Manual' | 'Campanha' | 'Outros';

/**
 * Procedência de um recurso (imóvel/lead) compartilhado por uma imobiliária
 * parceira. Marca o recurso como "de terceiro" na lista própria do usuário
 * — usado para badge, aba "Compartilhados" e fluxo de co-corretagem.
 */
export interface SharedFrom {
 /** ID da parceria que concede o acesso. */
 partnershipId: string;
 /** ID da empresa dona do recurso (captadora). */
 companyId: string;
 /** Nome da empresa dona — resolvido no frontend via getCompany(). */
 companyName: string;
}

export interface Lead {
 document?: string;
 id: string;
 displayId?: string;
 name: string;
 email: string;
 phone: string;
 property: string;
 budget: number;
 status: LeadStatus;
 avatar?: string;
 date: string;
 agentId?: string;
 agentName?: string;
  role?: string;
  isClient?: boolean;
  accountId?: string;
  leadType?: LeadType;
 source?: LeadSource;
 /**
  * Quando presente, este lead foi compartilhado por uma imobiliária
  * parceira (co-corretagem). Dados de contato podem vir em branco por
  * projeção de privacidade do backend.
  */
 sharedFrom?: SharedFrom;
}

export type PropertyStatus =
 | 'Em Captação'
 | 'Em Portfólio'
 | 'Anunciado'
 | 'Em Atendimento'
 | 'Em Negociação'
 | 'Em Proposta'
 | 'Em Análise'
 | 'Em Contrato'
 | 'Alugado'
 | 'Vendido'
 | 'Em Administração'
 | 'Em Rescisão'
 | 'Disponível'
 | 'Inativo';

export interface PropertyAddress {
 street: string;
 number: string;
 complement?: string;
 neighborhood: string;
 city: string;
 state: string;
 stateCode: string;
 postalCode: string;
 country?: string;
 countryCode?: string;
 /** Coordenadas geográficas (string para compat com o wire do backend). */
 latitude?: string;
 longitude?: string;
}

export function formatPropertyAddress(address: PropertyAddress): string {
 const parts = [
 address.street,
 address.number,
 address.complement,
 ].filter(Boolean).join(', ');
 return `${parts} - ${address.neighborhood}, ${address.city} - ${address.stateCode}`;
}

export function resolvePropertyDisplay(
 value: string | undefined,
 properties: Property[],
): string {
 if (!value) return 'Interesse Geral';
 const property = properties.find((p) => p.id === value);
 if (property) {
  if (property.title?.trim()) return property.title;
  if (property.address) return formatPropertyAddress(property.address);
  return property.displayId || property.id;
 }
 return value;
}

export interface Property {
 id: string;
 displayId?: string;
 title: string;
 description?: string;
 address: PropertyAddress;
 price: number;
 salePrice?: number;
 rentPrice?: number;
 condominiumFee?: number;
 propertyTax?: number;
 type: string;
 status: PropertyStatus;
 beds: number;
 baths: number;
 suites?: number;
 parkingSpaces?: number;
 area: number;
 image: string;
 images?: { url: string; description?: string; order?: number; filename?: string }[];
 videos?: { url: string; platform?: 'youtube' | 'vimeo' | 'other' }[];
 amenities?: string[];
 ownerId?: string;
 owners?: { id: string; percentage?: number }[];
 capturingBrokers?: { id: string; role: string; creci?: string; commissionPct?: number }[];
 intermediationContractId?: string;
 availableFor?: string[];
 interestedLeads?: { id: string; date: string; source?: string }[];
 activeDealId?: string;
 partnershipPortfolio?: boolean;
 /** Exclusividade da captação: 'open' | 'exclusive'. */
 captureExclusivity?: 'open' | 'exclusive';
 documents?: { name: string; createdAt: string; status: string; type: string }[];
 publicationStatus?: 'draft' | 'published';
 listingType?: 'sale' | 'rent' | 'exchange';
 condition?: 'new' | 'used' | 'under_construction';
 views?: number;
 leadsGenerated?: number;
 /**
  * Quando presente, este imóvel foi compartilhado por uma imobiliária
  * parceira. O backend já aplica projeção de privacidade (dados do
  * proprietário removidos, endereço reduzido ao bairro, lat/long
  * arredondados). Habilita a aba "Compartilhados" e a co-corretagem.
  */
 sharedFrom?: SharedFrom;
}

export interface ChatMessage {
 id: string;
 text: string;
 sender: 'me' | 'other';
 time: string;
}

export interface Conversation {
 id: string;
 contactName: string;
 avatar: string;
 lastMessage: string;
 time: string;
 unread: number;
 messages: ChatMessage[];
 propertyInterest: string;
}

export type AppointmentType = 'Visita' | 'Reuniao' | 'Reunião' | 'Ligacao' | 'Assinatura' | 'Vistoria' | 'Lembrete' | 'Outro' | 'Entrega';
export type AppointmentStatus = 'Pendente' | 'Confirmado' | 'Concluido' | 'Cancelado' | 'Reagendado' | 'Agendado' | 'Em Andamento';
export type AppointmentPriority = 'Alta' | 'Media' | 'Baixa';

export interface Appointment {
 inspectionId?: string;
 inspectionContractId?: string;
 inspectorId?: string;
 inspectorName?: string;
 id: string;
 title: string;
 client: string;
 type: AppointmentType;
 date: string; // ISO YYYY-MM-DD
 time: string; // HH:mm
 endTime: string; // HH:mm
 location: string;
 status: AppointmentStatus;
 priority: AppointmentPriority;
 description?: string;
 notes?: string;
 agentId?: string;
 agentName?: string;
 visibility?: 'team' | 'private';
 propertyId?: string;
 contractId?: string;
 leadId?: string;
 requestedByClient?: boolean;
 customerAccountId?: string;
 isAllDay?: boolean;
 reminder?: number; // minutes before
 createdAt: string;
 updatedAt?: string;
}

// ── Document Checklist Types ──────────────────────────────────────

export type ChecklistItemStatus = 'pending' | 'uploaded' | 'approved' | 'rejected';

export type ChecklistItemCategory = 'pessoal' | 'financeiro' | 'imovel' | 'juridico' | 'outros';

export const CHECKLIST_CATEGORY_LABELS: Record<ChecklistItemCategory, string> = {
 pessoal: 'Pessoal',
 financeiro: 'Financeiro',
 imovel: 'Imóvel',
 juridico: 'Jurídico',
 outros: 'Outros',
};

export interface DocumentChecklistItem {
 id: string;
 name: string;
 category: ChecklistItemCategory;
 partyId?: string;
 status: ChecklistItemStatus;
 required: boolean;
 description?: string;
 requestMode?: "upload" | "form";
 requestedFields?: Array<{ key: string; label: string; type: string; required: boolean }>;
 rejectionReason?: string;
 uploadedAt?: string;
 reviewedAt?: string;
 ocrValidated?: boolean;
 fileUrl?: string;
 fileName?: string;
 solicited?: boolean;
 solicitedAt?: string;
 /** Snapshot do formulário no momento do envio pelo cliente. */
 formSubmission?: {
  values: Record<string, string>;
  requestedFields?: Array<{ key: string; label: string; type: string; required: boolean }>;
  submittedBy: string;
  submittedAt: string;
 };
}

export type ChecklistTemplateId = 'pf_compra_financiamento' | 'pf_compra_avista' | 'pj_compra' | 'locacao_pf' | 'locacao_pj';

export interface ChecklistTemplateItem {
 name: string;
 category: ChecklistItemCategory;
 required: boolean;
 description?: string;
 /** "form" = apenas preenchimento de formulário (sem anexo), ex. CPF/telefone. */
 requestMode?: "upload" | "form";
 requestedFields?: Array<{ key: string; label: string; type: string; required: boolean }>;
}

export interface ChecklistTemplate {
 id: ChecklistTemplateId;
 label: string;
 items: ChecklistTemplateItem[];
}

export const DEFAULT_CHECKLIST_TEMPLATES: ChecklistTemplate[] = [
 {
 id: 'pf_compra_financiamento',
 label: 'PF — Compra com Financiamento',
 items: [
 { name: 'Documento de Identidade (RG/CNH)', category: 'pessoal', required: true },
 { name: 'CPF', category: 'pessoal', required: true, requestMode: 'form', requestedFields: [{ key: 'cpf', label: 'CPF', type: 'text', required: true }] },
 { name: 'Certidão de Nascimento ou Casamento', category: 'pessoal', required: true },
 { name: 'Comprovante de Residência', category: 'pessoal', required: true },
 { name: 'Comprovante de Renda (3 últimos meses)', category: 'financeiro', required: true },
 { name: 'Declaração de Imposto de Renda Completa', category: 'financeiro', required: true },
 { name: 'Extrato do FGTS', category: 'financeiro', required: true },
 { name: 'Carteira de Trabalho (páginas de identificação)', category: 'financeiro', required: false },
   { name: 'Telefone/WhatsApp', category: 'pessoal', required: false, requestMode: 'form', requestedFields: [{ key: 'phone', label: 'Telefone/WhatsApp', type: 'text', required: true }] },
 ],
 },
 {
 id: 'pf_compra_avista',
 label: 'PF — Compra à Vista',
 items: [
 { name: 'Documento de Identidade (RG/CNH)', category: 'pessoal', required: true },
 { name: 'CPF', category: 'pessoal', required: true, requestMode: 'form', requestedFields: [{ key: 'cpf', label: 'CPF', type: 'text', required: true }] },
 { name: 'Certidão de Nascimento ou Casamento', category: 'pessoal', required: true },
 { name: 'Comprovante de Residência', category: 'pessoal', required: true },
 { name: 'Comprovante de Renda ou Declaração de IR', category: 'financeiro', required: true },
 ],
 },
 {
 id: 'pj_compra',
 label: 'PJ — Compra',
 items: [
 { name: 'Contrato Social e Alterações', category: 'juridico', required: true },
 { name: 'Cartão CNPJ', category: 'juridico', required: true },
 { name: 'Procuração do Representante Legal', category: 'juridico', required: true },
 { name: 'Documentos dos Sócios (RG/CPF)', category: 'pessoal', required: true },
 { name: 'Balanço Patrimonial (último exercício)', category: 'financeiro', required: true },
 { name: 'Certidões Negativas (Federal, Estadual, Municipal)', category: 'juridico', required: true },
 ],
 },
 {
 id: 'locacao_pf',
 label: 'PF — Locação',
 items: [
 { name: 'Documento de Identidade (RG/CNH)', category: 'pessoal', required: true },
 { name: 'CPF', category: 'pessoal', required: true, requestMode: 'form', requestedFields: [{ key: 'cpf', label: 'CPF', type: 'text', required: true }] },
 { name: 'Comprovante de Residência Atual', category: 'pessoal', required: true },
 { name: 'Comprovante de Renda (3 últimos meses)', category: 'financeiro', required: true },
 { name: 'Documentos do Fiador (RG, CPF, Renda, Matrícula do Imóvel)', category: 'pessoal', required: false, description: 'Caso a garantia seja fiança' },
   { name: 'Telefone/WhatsApp', category: 'pessoal', required: false, requestMode: 'form', requestedFields: [{ key: 'phone', label: 'Telefone/WhatsApp', type: 'text', required: true }] },
 ],
 },
 {
 id: 'locacao_pj',
 label: 'PJ — Locação',
 items: [
 { name: 'Contrato Social e Última Alteração', category: 'juridico', required: true },
 { name: 'Cartão CNPJ', category: 'juridico', required: true },
 { name: 'Balanço Patrimonial ou DRE', category: 'financeiro', required: true },
 { name: 'Documentos dos Sócios (RG/CPF)', category: 'pessoal', required: true },
 { name: 'Comprovante de Endereço da Empresa', category: 'pessoal', required: true },
 ],
 },
];

/**
 * Sugestões de comodidades (lista curada, PT-BR). O campo é livre — as
 * sugestões existem para o operador escrever igual dos dois lados (a busca e o
 * contrato usam o texto cadastrado) e evitar "piscina"/"Piscina"/"piscina ".
 */
export const AMENITY_SUGGESTIONS: string[] = [
  'Piscina', 'Piscina aquecida', 'Academia', 'Salão de festas', 'Churrasqueira',
  'Playground', 'Quadra poliesportiva', 'Portaria 24h', 'Elevador', 'Varanda gourmet',
  'Sacada', 'Área de serviço', 'Armários planejados', 'Ar-condicionado', 'Mobiliado',
  'Aceita pets', 'Gás encanado', 'Gerador', 'Bicicletário', 'Coworking',
  'Salão de jogos', 'Sauna', 'Espaço gourmet', 'Condomínio fechado', 'Segurança 24h',
  'Interfone', 'Vista para o mar', 'Quintal', 'Edícula', 'Poço artesiano',
];
