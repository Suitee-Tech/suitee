import { t } from "../lib/i18n";
import { resolveDocumentOrigin, type DocumentOriginId } from "./origins";

/**
 * Uma referência é o "endereço" legível de um documento dentro da operação:
 * o contrato, o imóvel, o cliente, o negócio, a vistoria ou o chamado ao qual
 * ele pertence. A Central de Documentos mostra isso em vez de UUIDs crus.
 */
export interface DocumentReference {
  origin: DocumentOriginId;
  /** Chave do registro dentro da origem (contrato/imóvel/cliente...). */
  entityId: string;
  /** Rótulo curto e legível: "Vanessa Rodrigues · Apartamento dos Ipê". */
  label: string;
  /**
   * Rótulo do *registro* (não do arquivo) usado para agrupar na barra
   * lateral. Em análises de crédito, por exemplo, vários documentos do mesmo
   * negócio têm rótulos próprios ("CND · Fulano", "Score · Beltrano") mas
   * precisam aparecer sob um único nó do negócio.
   */
  groupLabel: string;
  /** Complemento para tooltip, ex.: "Contrato CTR-2026-014". */
  detail: string;
  /** Complemento do nó agrupado, quando difere do `detail` do arquivo. */
  groupDetail?: string;
  /** Rota interna do registro, quando existir. */
  href?: string;
}

export type ActorKind = "person" | "system" | "unknown";

export interface DocumentActor {
  /** Nome legível de quem enviou o arquivo. */
  label: string;
  /** Valor original, usado em tooltip. */
  detail?: string;
  kind: ActorKind;
}

export interface ReferenceContract {
  id: string;
  display_id?: string;
  client?: string;
  property_title?: string;
  property_id?: string;
  deal_id?: string;
}

export interface ReferenceProperty {
  id: string;
  title?: string;
  displayId?: string;
}

export interface ReferenceLead {
  id: string;
  name?: string;
  accountId?: string;
  displayId?: string;
}

export interface ReferenceDeal {
  id: string;
  display_id?: string;
  property_id?: string;
  customer_id?: string;
  lead_id?: string;
  parties?: readonly { id: string; name?: string }[];
}

export interface ReferenceInspection {
  id: string;
  property_id?: string;
  contract_id?: string;
  type?: string;
  status?: string;
  scheduled_date?: string;
}

export interface ReferenceTicket {
  id: string;
  property_id?: string;
  property_title?: string;
  type?: string;
  category?: string;
  status?: string;
}

export interface ReferenceMember {
  account_id: string;
  name?: string;
  email?: string;
}

export interface DocumentReferenceSources {
  contracts?: readonly ReferenceContract[];
  properties?: readonly ReferenceProperty[];
  leads?: readonly ReferenceLead[];
  deals?: readonly ReferenceDeal[];
  inspections?: readonly ReferenceInspection[];
  tickets?: readonly ReferenceTicket[];
  members?: readonly ReferenceMember[];
  /** Corretores já carregados na sessão (fallback quando não há membros). */
  agents?: readonly { id: string; name: string }[];
}

/** Documento visto apenas pelo que a referência precisa. */
export interface ReferenceDocument {
  entityId?: string | null;
  entityType?: string | null;
  contractId?: string | null;
  analysisPartyId?: string | null;
  analysisCriterion?: string | null;
}

const UUID_LIKE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function isOpaqueId(value: string): boolean {
  return UUID_LIKE.test(value) || /^[0-9a-f]{16,}$/i.test(value);
}

/** Encurta identificadores técnicos para algo que caiba na interface. */
export function shortIdentifier(value: string | null | undefined): string {
  const raw = (value ?? "").trim();
  if (!raw) return "";
  if (UUID_LIKE.test(raw)) return raw.slice(0, 8);
  if (raw.length <= 12) return raw;
  return raw.slice(0, 10);
}

function firstNonEmpty(...values: (string | undefined | null)[]): string {
  for (const value of values) {
    const trimmed = (value ?? "").trim();
    if (trimmed) return trimmed;
  }
  return "";
}

export interface DocumentReferenceResolver {
  resolve(doc: ReferenceDocument): DocumentReference;
  resolveActor(raw: string | null | undefined): DocumentActor;
  /** Rótulo legível de um imóvel do acervo. */
  propertyLabel(id: string | null | undefined): string;
  /** Rótulo legível de um cliente/lead do acervo. */
  leadLabel(id: string | null | undefined): string;
}

/**
 * Monta um resolvedor a partir dos dados já carregados na sessão (contratos,
 * imóveis, clientes, negócios, vistorias, chamados e equipe). Nada é buscado
 * aqui — só cruzamos o que a aplicação já tem em memória.
 */
export function createDocumentReferenceResolver(
  sources: DocumentReferenceSources = {},
): DocumentReferenceResolver {
  const { contracts = [], properties = [], leads = [], deals = [], inspections = [], tickets = [], members = [], agents = [] } = sources;

  const contractById = new Map(contracts.map((c) => [c.id, c]));
  const propertyById = new Map(properties.map((p) => [p.id, p]));
  const leadById = new Map(leads.map((l) => [l.id, l]));
  const dealById = new Map(deals.map((d) => [d.id, d]));
  const inspectionById = new Map(inspections.map((i) => [i.id, i]));
  const ticketById = new Map(tickets.map((t) => [t.id, t]));

  const leadByAccountId = new Map<string, ReferenceLead>();
  for (const lead of leads) {
    if (lead.accountId) leadByAccountId.set(lead.accountId, lead);
  }

  const memberByAccountId = new Map<string, ReferenceMember>();
  const memberByEmail = new Map<string, ReferenceMember>();
  for (const member of members) {
    if (member.account_id) memberByAccountId.set(member.account_id, member);
    if (member.email) memberByEmail.set(member.email.trim().toLowerCase(), member);
  }

  const agentById = new Map(agents.map((a) => [a.id, a]));

  const propertyLabel = (id: string | null | undefined): string => {
    const key = (id ?? "").trim();
    if (!key) return "";
    const property = propertyById.get(key);
    return firstNonEmpty(property?.title, property?.displayId, `Imóvel ${shortIdentifier(key)}`);
  };

  const leadLabel = (id: string | null | undefined): string => {
    const key = (id ?? "").trim();
    if (!key) return "";
    const lead = leadById.get(key);
    return firstNonEmpty(lead?.name, lead?.displayId, `Cliente ${shortIdentifier(key)}`);
  };

  const dealLabel = (deal: ReferenceDeal | undefined, id: string): string => {
    if (!deal) return `Negócio ${shortIdentifier(id)}`;
    const client = leadLabel(deal.customer_id || deal.lead_id);
    const property = propertyLabel(deal.property_id);
    const parts = [client, property].filter(Boolean);
    if (parts.length > 0) return parts.join(" · ");
    return firstNonEmpty(deal.display_id, `Negócio ${shortIdentifier(id)}`);
  };

  const resolveActor = (raw: string | null | undefined): DocumentActor => {
    const value = (raw ?? "").trim();
    if (!value) return { label: "—", kind: "unknown" };

    const lower = value.toLowerCase();
    if (lower.startsWith("system:")) {
      const scope = value.slice("system:".length);
      const scopeLabel = scope
        .split(/[-_]/)
        .filter(Boolean)
        .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
        .join(" ");
      return { label: `Sistema · ${scopeLabel || "Automático"}`, detail: value, kind: "system" };
    }
    if (lower === "system") return { label: "Sistema", detail: value, kind: "system" };
    if (lower === "clicksign") return { label: "Clicksign", detail: "Assinatura digital", kind: "system" };

    const member = memberByAccountId.get(value) ?? memberByEmail.get(lower);
    if (member) {
      const name = firstNonEmpty(member.name, member.email);
      return { label: name, detail: value, kind: "person" };
    }

    const lead = leadByAccountId.get(value);
    if (lead?.name) return { label: `${lead.name} (cliente)`, detail: value, kind: "person" };

    const agent = agentById.get(value);
    if (agent?.name) return { label: agent.name, detail: value, kind: "person" };

    if (value.includes("@")) return { label: value, kind: "person" };
    if (isOpaqueId(value)) return { label: `Conta ${shortIdentifier(value)}`, detail: value, kind: "unknown" };

    return { label: value, kind: "unknown" };
  };

  const resolve = (doc: ReferenceDocument): DocumentReference => {
    const entityId = firstNonEmpty(doc.entityId);
    const contractId = firstNonEmpty(doc.contractId);
    const origin = resolveDocumentOrigin({
      entityType: doc.entityType,
      entityId: doc.entityId,
      contractId,
    });

    switch (origin) {
      case "contract": {
        const id = entityId || contractId;
        const contract = contractById.get(id);
        const label = firstNonEmpty(
          contract ? [contract.client, contract.property_title].filter(Boolean).join(" · ") : "",
          contract?.display_id,
          contract?.property_title,
          `Contrato ${shortIdentifier(id)}`,
        );
        return {
          origin,
          entityId: id,
          label,
          groupLabel: label,
          detail: `Contrato ${firstNonEmpty(contract?.display_id, shortIdentifier(id))}`,
          href: id ? `/contratos/${id}` : undefined,
        };
      }
      case "deal": {
        const deal = dealById.get(entityId);
        return {
          origin,
          entityId,
          label: dealLabel(deal, entityId),
          groupLabel: dealLabel(deal, entityId),
          detail: `Negócio ${firstNonEmpty(deal?.display_id, shortIdentifier(entityId))}`,
          href: entityId ? `/esteira/deals/${entityId}` : undefined,
        };
      }
      case "property": {
        return {
          origin,
          entityId,
          label: propertyLabel(entityId),
          groupLabel: propertyLabel(entityId),
          detail: `Imóvel ${firstNonEmpty(propertyById.get(entityId)?.displayId, shortIdentifier(entityId))}`,
          href: entityId ? `/imoveis/${entityId}` : undefined,
        };
      }
      case "client": {
        const lead = leadById.get(entityId);
        return {
          origin,
          entityId,
          label: leadLabel(entityId),
          groupLabel: leadLabel(entityId),
          detail: `Cliente ${firstNonEmpty(lead?.displayId, shortIdentifier(entityId))}`,
          href: entityId ? `/leads/${entityId}` : undefined,
        };
      }
      case "inspection": {
        const inspection = inspectionById.get(entityId);
        const property = propertyLabel(inspection?.property_id);
        const type = firstNonEmpty(inspection?.type, "Vistoria");
        const label = firstNonEmpty(
          property ? `${type} · ${property}` : "",
          `Vistoria ${shortIdentifier(entityId)}`,
        );
        return {
          origin,
          entityId,
          label,
          groupLabel: label,
          detail: "Vistoria",
          href: inspection?.contract_id ? `/contratos/${inspection.contract_id}` : property ? "/locacao" : undefined,
        };
      }
      case "maintenance": {
        const ticket = ticketById.get(entityId);
        const property = firstNonEmpty(ticket?.property_title, propertyLabel(ticket?.property_id));
        const label = firstNonEmpty(
          property ? `${firstNonEmpty(ticket?.category, ticket?.type, "Chamado")} · ${property}` : "",
          `Chamado ${shortIdentifier(entityId)}`,
        );
        return {
          origin,
          entityId,
          label,
          groupLabel: label,
          detail: "Chamado de manutenção",
          href: "/chamados",
        };
      }
      case "analysis": {
        const deal = dealById.get(entityId);
        const partyId = firstNonEmpty(doc.analysisPartyId);
        const party =
          (partyId ? deal?.parties?.find((p) => p.id === partyId) : undefined) ??
          (partyId ? deal?.parties?.find((p) => p.name === partyId) : undefined);
        const criterion = firstNonEmpty(doc.analysisCriterion);
        const criterionLabel = criterion ? t(`documents.analysis.${criterion}`) : "";
        const context = firstNonEmpty(party?.name, deal ? dealLabel(deal, entityId) : "");
        return {
          origin,
          entityId,
          label: firstNonEmpty(
            [criterionLabel, context].filter(Boolean).join(" · "),
            dealLabel(deal, entityId),
            `Análise ${shortIdentifier(entityId)}`,
          ),
          // O nó da barra lateral é o negócio analisado; o critério e a parte
          // ficam no rótulo de cada arquivo.
          groupLabel: firstNonEmpty(dealLabel(deal, entityId), `Análise ${shortIdentifier(entityId)}`),
          groupDetail: "Análise de crédito do negócio",
          detail: criterionLabel
            ? `Análise de crédito · ${criterionLabel}`
            : "Análise de crédito (due diligence)",
          href: entityId ? `/esteira/deals/${entityId}` : undefined,
        };
      }
      case "checklist": {
        const label = `Checklist ${shortIdentifier(entityId)}`;
        return { origin, entityId, label, groupLabel: label, detail: "Item da esteira digital" };
      }
      case "fiscal": {
        const label = entityId ? `Fiscal ${shortIdentifier(entityId)}` : "Fiscal";
        return { origin, entityId, label, groupLabel: label, detail: "Documento fiscal", href: "/fiscal" };
      }
      case "financial": {
        const label = entityId ? `Lançamento ${shortIdentifier(entityId)}` : "Financeiro";
        return { origin, entityId, label, groupLabel: label, detail: "Documento financeiro", href: "/financeiro" };
      }
      default:
        return {
          origin: "unassigned",
          entityId,
          label: "Sem vínculo operacional",
          groupLabel: "Sem vínculo operacional",
          detail: "Arquivo enviado sem vínculo com contrato, imóvel ou cliente",
        };
    }
  };

  return { resolve, resolveActor, propertyLabel, leadLabel };
}
