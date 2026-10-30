import { CONTRACT_DATE_BASE_KEYS, resolveContractDateVariable } from "../contracts/contractDateVariables";
import { PARTY_QUALIFICATION_FIELDS, type PartyLike } from "./contractParties";
import type { Contract } from "../contracts/types";
import type { FirstChargeType } from "../rentals/types";
import type { ApiLead } from "../services/lead.service";
import type { Company } from "../services/auth.service";
import type { BrokerDTO } from "../services/broker.service";
import type { DealDTO, Proposal, Signer } from "../services/deal.service";
import type { PropertyDTO } from "../services/property.service";
import { buildRoleLegacy as buildRoleLegacyUtil, toPartyLoopItem as toPartyLoopItemUtil } from "../utils/contractParties";
import { capitalizeFirst, currencyToWordsBRL, formatCurrencyBRL } from "../utils/format";

interface ContractVariableDataArgs {
  dealDetails: DealDTO | null;
  property: PropertyDTO | null;
  signers: Signer[];
  buyerLead: ApiLead | null;
  sellerLead: ApiLead | null;
  brokersById: Record<string, BrokerDTO>;
  ownerLeadsById: Record<string, ApiLead>;
  rentalStartDate: string;
  rentalEndDate: string;
  computedEndDate: string;
  firstChargeDate: string;
  firstChargeType: FirstChargeType;
  defaultDueDay: number;
  contract: Contract | null;
  preferContractDates?: boolean;
  company?: Company | null;
}

export function buildContractVariableData({
  dealDetails,
  property,
  signers,
  buyerLead,
  sellerLead,
  brokersById,
  ownerLeadsById,
  rentalStartDate,
  rentalEndDate,
  computedEndDate,
  firstChargeDate,
  firstChargeType,
  defaultDueDay,
  contract,
  preferContractDates = false,
  company,
}: ContractVariableDataArgs): Record<string, any> {
  if (!dealDetails) return {};

  const PLACEHOLDER = "______________________";
  const companyAddress = company?.address
    ? [
        [company.address.street, company.address.number].filter(Boolean).join(", "),
        company.address.complement,
        company.address.district,
        [company.address.city, company.address.state].filter(Boolean).join(" - "),
        company.address.zip_code ? `CEP ${company.address.zip_code}` : "",
      ].filter(Boolean).join(", ")
    : "";
  const isRentalDeal = dealDetails.type === "RENT";
  const now = new Date();
  const currentDateIso = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;

  let formattedAddress = "";
  if (property?.address) {
    const { street, number, complement, neighborhood, city, state } = property.address;
    const postal = (property.address as any).postalCode ?? (property.address as any).postal_code ?? "";
    formattedAddress = property.address.formatted_address || [[street, number].filter(Boolean).join(", "), complement, neighborhood, [city, state].filter(Boolean).join(" - "), postal ? `CEP ${postal}` : ""].filter(Boolean).join(", ");
  }

  const sourceParties = dealDetails.parties || [];
  const parties = sourceParties.map((party) => {
    const spouse = sourceParties.find((other) => other.role === "spouse" && other.spouse_of_party_id === party.id)
      || sourceParties.find((other) => other.id === party.spouse_of_party_id);
    const representative = sourceParties.find((other) => other.id === party.representation?.representative_party_id);
    const represented = sourceParties.filter((other) => other.representation?.representative_party_id === party.id);
    const lead = party.lead_id ? ownerLeadsById[party.lead_id]
      || (buyerLead?.id === party.lead_id ? buyerLead : null)
      || (sellerLead?.id === party.lead_id ? sellerLead : null) : null;
    return {
      ...party,
      ...toPartyLoopItemUtil({ ...party, qualification: lead?.qualification,
        spouse_name: spouse?.name || "", spouse_document: spouse?.document || "", spouse_email: spouse?.email || "",
        representative_name: representative?.name || "", representative_document: representative?.document || "", representative_email: representative?.email || "",
        represented_party_names_list: represented.map((item) => item.name).filter(Boolean).join("; "),
      }, PLACEHOLDER),
      name: party.name || lead?.name || "",
      email: party.email || lead?.email || "",
      phone: party.phone || lead?.phone || "",
      document: party.document || lead?.document || "",
    };
  });
  const tenants = parties.filter((p) => p.role === "tenant");
  const buyers = parties.filter((p) => p.role === "buyer");
  const owners = parties.filter((p) => p.role === "owner");
  const guarantors = parties.filter((p) => p.role === "guarantor");
  const witnesses = parties.filter((p) => p.role === "witness");
  const spouses = parties.filter((p) => p.role === "spouse");
  const attorneys = parties.filter((p) => p.role === "attorney");
  const legalReps = parties.filter((p) => p.role === "legal_representative");
  const guardians = parties.filter((p) => p.role === "guardian");
  const curators = parties.filter((p) => p.role === "curator");

  const primaryTenant = tenants.find((p) => p.is_primary) || tenants[0];
  const primaryBuyer = buyers.find((p) => p.is_primary) || buyers[0];
  const primaryOwner = owners.find((p) => p.is_primary) || owners[0];

  const buyerSigner = signers.find((s) => s.role.toLowerCase() === "comprador" || s.role.toLowerCase() === "inquilino");
  const sellerSigner = signers.find((s) => s.role.toLowerCase() === "vendedor" || s.role.toLowerCase() === "proprietario");

  const buyerParty = isRentalDeal ? primaryTenant : primaryBuyer;
  const sellerParty = primaryOwner;
  const sellerRoles = parties.filter((p) => p.role === "seller");
  const landlordRoles = parties.filter((p) => p.role === "landlord");
  const sellerList = sellerRoles.length > 0 ? sellerRoles : sellerParty ? [sellerParty] : [];
  const landlordList = landlordRoles.length > 0 ? landlordRoles : isRentalDeal ? owners : [];
  const primarySeller = sellerRoles.find((p) => p.is_primary) || sellerRoles[0] || sellerParty;
  const primaryLandlord = landlordRoles.find((p) => p.is_primary) || landlordRoles[0] || (isRentalDeal ? primaryOwner : undefined);

  const buyerName = buyerParty?.name || buyerSigner?.name || buyerLead?.name || PLACEHOLDER;
  const buyerEmail = buyerParty?.email || buyerSigner?.email || buyerLead?.email || PLACEHOLDER;
  const buyerPhone = buyerParty?.phone || buyerLead?.phone || PLACEHOLDER;
  const buyerDocument = buyerParty?.document || buyerLead?.document || PLACEHOLDER;

  const sellerName = sellerParty?.name || sellerSigner?.name || sellerLead?.name || PLACEHOLDER;
  const sellerEmail = sellerParty?.email || sellerSigner?.email || sellerLead?.email || PLACEHOLDER;
  const sellerPhone = sellerParty?.phone || sellerLead?.phone || PLACEHOLDER;
  const sellerDocument = sellerParty?.document || sellerLead?.document || PLACEHOLDER;

  const proposal: Proposal | undefined = dealDetails.proposals?.[0];
  const dealValue = dealDetails.value || 0;
  const downPayment = proposal?.down_payment || 0;
  const installments = proposal?.installments || 1;
  const financedValue = Math.max(dealValue - downPayment, 0);
  const installmentValue = installments > 0 ? financedValue / installments : 0;

  const commissionPct = dealDetails.commission || 0;
  const commissionValue = (dealValue * commissionPct) / 100;

  const firstChargeTypeLabel = firstChargeType === "same_month_pro_rata" ? "Mesmo mês (pro rata)" : "Próximo mês (acumulado)";

  const rentalEndBase = rentalEndDate || computedEndDate;

  const safeIso = (value?: string | null): string =>
    resolveContractDateVariable({ current_date: value }, "current_date_iso") || "";
  const dateBR = (value?: string | null): string =>
    resolveContractDateVariable({ current_date: value }, "current_date") || "";

  const draftStart = rentalStartDate;
  const draftEnd = rentalEndBase;
  const contractStartIso = isRentalDeal
    ? safeIso(preferContractDates && contract ? contract.start_date : draftStart)
    : safeIso(contract?.start_date) || currentDateIso;
  const contractEndIso = isRentalDeal
    ? safeIso(preferContractDates && contract ? contract.end_date : draftEnd)
    : safeIso(contract?.end_date);
  const firstChargeIso = isRentalDeal ? safeIso(firstChargeDate) : "";

  const computeNextDueDate = (baseDate: string, dueDay: number): string => {
    if (!baseDate || !Number.isInteger(dueDay) || dueDay < 1 || dueDay > 31) return "";
    const base = new Date(`${baseDate}T00:00:00Z`);
    const candidate = (monthOffset: number) => {
      const year = base.getUTCFullYear();
      const month = base.getUTCMonth() + monthOffset;
      const lastDay = new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
      return new Date(Date.UTC(year, month, Math.min(dueDay, lastDay)));
    };
    const next = candidate(0) > base ? candidate(0) : candidate(1);
    return next.toISOString().slice(0, 10);
  };
  const standardDueIso = isRentalDeal && firstChargeIso ? computeNextDueDate(firstChargeIso, defaultDueDay) : "";
  const contractTermMonths = (() => {
    if (!contractStartIso || !contractEndIso || contractEndIso < contractStartIso) return "";
    const start = new Date(`${contractStartIso}T00:00:00Z`);
    const end = new Date(`${contractEndIso}T00:00:00Z`);
    let months = (end.getUTCFullYear() - start.getUTCFullYear()) * 12 + end.getUTCMonth() - start.getUTCMonth();
    const anniversary = resolveContractDateVariable({ contract_start_date: contractStartIso }, `contract_start_date_plus_${months}_months_iso`);
    if (anniversary && anniversary > contractEndIso) months--;
    return String(Math.max(0, months));
  })();

  const propertyTypeLabel =
    property?.property_type === "apartment"
      ? "Apartamento"
      : property?.property_type === "house"
        ? "Casa"
        : property?.property_type === "commercial"
          ? "Comercial"
          : property?.property_type === "land"
            ? "Terreno"
            : "";

  const listingTypeLabel =
    property?.listing_type === "sale" ? "Venda" : property?.listing_type === "rent" ? "Locação" : property?.listing_type === "exchange" ? "Permuta" : "";

  const propertySalePrice = property?.pricing?.sale_price?.current_amount ?? null;
  const propertyRentPrice = property?.pricing?.rent_price?.current_amount ?? null;
  const propertyCondoFee = property?.pricing?.condominium_fee?.current_amount ?? null;
  const propertyPropertyTax = property?.pricing?.property_tax?.current_amount ?? null;

  const dd = dealDetails.due_diligence;
  const checklist = dealDetails.checklist || [];
  const checklistPending = checklist.filter((i) => i.status === "PENDING" || i.status === "UPLOADED").length;
  const checklistApproved = checklist.filter((i) => i.status === "APPROVED").length;
  const checklistRejected = checklist.filter((i) => i.status === "REJECTED").length;
  const checklistPendingRequired = checklist.filter((i) => i.required && (i.status === "PENDING" || i.status === "UPLOADED")).length;

  const primaryBroker = dealDetails.broker_id ? brokersById[dealDetails.broker_id] : undefined;
  const participations = dealDetails.broker_participations || [];
  const captador = participations.find((p) => p.role === "captador");
  const visitador = participations.find((p) => p.role === "visitador");
  const fechamento = participations.find((p) => p.role === "fechamento");

  const participationToText = (p: typeof captador) => {
    if (!p) return "";
    const b = brokersById[p.broker_id];
    const name = b?.name || p.broker_id;
    const pct = typeof p.percentage === "number" ? `${p.percentage}%` : typeof p.commission === "number" ? `${p.commission}%` : "";
    return pct ? `${name} (${pct})` : name;
  };

  const brokerParticipationsSummary =
    [captador ? `captador: ${participationToText(captador)}` : "", visitador ? `visitador: ${participationToText(visitador)}` : "", fechamento ? `fechamento: ${participationToText(fechamento)}` : ""]
      .filter(Boolean)
      .join(" | ") || PLACEHOLDER;

  const propertyOwners = property?.owners?.length ? property.owners : property?.owner ? [property.owner] : [];
  const propertyOwnerIds = propertyOwners.map((o) => o.id).filter(Boolean);
  const propertyOwnerNames =
    propertyOwners
      .map((o) => {
        const lead = ownerLeadsById[o.id];
        const name = lead?.name || o.id;
        const pct = typeof o.percentage === "number" ? ` (${o.percentage}%)` : "";
        return `${name}${pct}`;
      })
      .filter(Boolean)
      .join("; ") || PLACEHOLDER;

  const capturing = property?.capturing_brokers || [];
  const capturingSummary =
    capturing
      .map((c) => {
        const b = brokersById[c.id];
        const name = b?.name || c.id;
        return `${c.role}: ${name}`;
      })
      .join(" | ") || PLACEHOLDER;

  const buyerObj = toPartyLoopItemUtil({
    ...buyerParty,
    qualification: !buyerParty?.lead_id || buyerParty.lead_id === buyerLead?.id ? buyerLead?.qualification : undefined,
    id: buyerParty?.id || buyerLead?.id || "",
    name: buyerName, email: buyerEmail, phone: buyerPhone, document: buyerDocument,
  }, PLACEHOLDER);
  const sellerObj = toPartyLoopItemUtil({
    ...sellerParty,
    qualification: !sellerParty?.lead_id || sellerParty.lead_id === sellerLead?.id ? sellerLead?.qualification : undefined,
    id: sellerParty?.id || sellerLead?.id || "",
    name: sellerName, email: sellerEmail, phone: sellerPhone, document: sellerDocument,
  }, PLACEHOLDER);
  const tenantObj = isRentalDeal ? buyerObj : toPartyLoopItemUtil({}, PLACEHOLDER);
  const landlordObj = isRentalDeal ? sellerObj : toPartyLoopItemUtil({}, PLACEHOLDER);
  const partiesArray = parties.map((party) => toPartyLoopItemUtil(party, PLACEHOLDER));
  const scalarQualifications = (prefix: string, party: typeof buyerObj) => Object.fromEntries(
    PARTY_QUALIFICATION_FIELDS.map((field) => [`${prefix}_${field}`, party[field]]),
  );
  const brokerRoleAliases = Object.fromEntries([
    ["listing", captador], ["visiting", visitador], ["closing", fechamento],
  ].flatMap(([role, participation]) => {
    const p = participation as typeof captador;
    const broker = p ? brokersById[p.broker_id] : undefined;
    return [[`broker_${role}_id`, p?.broker_id || ""], [`broker_${role}_name`, broker?.name || p?.broker_id || ""], [`broker_${role}_email`, broker?.email || ""]];
  }));

  const toPartyLoopItem = (p: PartyLike) => toPartyLoopItemUtil(p, PLACEHOLDER);
  const buildRoleLegacy = (singular: string, plural: string, arr: PartyLike[], primary: PartyLike | undefined) =>
    buildRoleLegacyUtil(singular, plural, arr, primary, PLACEHOLDER);

  const tenantsLegacy = buildRoleLegacy("tenant", "tenants", tenants, primaryTenant);
  const buyersLegacy = buildRoleLegacy("buyer", "buyers", buyers, primaryBuyer);
  const ownersLegacy = buildRoleLegacy("owner", "owners", owners, primaryOwner);
  const guarantorsLegacy = buildRoleLegacy("guarantor", "guarantors", guarantors, guarantors.find((p) => p.is_primary) || guarantors[0]);
  const witnessesLegacy = buildRoleLegacy("witness", "witnesses", witnesses, witnesses.find((p) => p.is_primary) || witnesses[0]);
  const spousesLegacy = buildRoleLegacy("spouse", "spouses", spouses, spouses.find((p) => p.is_primary) || spouses[0]);
  const attorneysLegacy = buildRoleLegacy("attorney", "attorneys", attorneys, attorneys.find((p) => p.is_primary) || attorneys[0]);
  const legalRepsLegacy = buildRoleLegacy("legal_representative", "legal_representatives", legalReps, legalReps.find((p) => p.is_primary) || legalReps[0]);
  const guardiansLegacy = buildRoleLegacy("guardian", "guardians", guardians, guardians.find((p) => p.is_primary) || guardians[0]);
  const curatorsLegacy = buildRoleLegacy("curator", "curators", curators, curators.find((p) => p.is_primary) || curators[0]);
  const sellersLegacy = buildRoleLegacy("seller", "sellers", sellerList, primarySeller);
  const landlordsLegacy = buildRoleLegacy("landlord", "landlords", landlordList, primaryLandlord);

  const roleObject = (party: PartyLike | undefined) => toPartyLoopItemUtil(party || {}, PLACEHOLDER);
  const signerList = signers.map((signer) => ({
    name: signer.name,
    email: signer.email,
    role: signer.role,
    status: signer.status,
    signed_at: signer.signed_at ? dateBR(signer.signed_at) : "",
    is_required: signer.is_required === false ? "Não" : "Sim",
  }));

  const data: Record<string, unknown> = {
    buyer_address: buyerObj.address,

    tenant_address: tenantObj.address,

    seller_name: sellerName,
    seller_email: sellerEmail,
    seller_document: sellerDocument,
    seller_phone: sellerPhone,
    seller_address: sellerObj.address,

    landlord_name: isRentalDeal ? sellerName : PLACEHOLDER,
    landlord_email: isRentalDeal ? sellerEmail : PLACEHOLDER,
    landlord_document: isRentalDeal ? sellerDocument : PLACEHOLDER,
    landlord_phone: isRentalDeal ? sellerPhone : PLACEHOLDER,
    landlord_address: landlordObj.address,

    buyer: buyerObj,
    seller: sellerObj,
    tenant: tenantObj,
    landlord: landlordObj,
    parties: partiesArray,
    buyers: buyers.map(toPartyLoopItem),
    tenants: tenants.map(toPartyLoopItem),
    owners: owners.map(toPartyLoopItem),
    guarantors: guarantors.map(toPartyLoopItem),
    witnesses: witnesses.map(toPartyLoopItem),
    spouses: spouses.map(toPartyLoopItem),
    attorneys: attorneys.map(toPartyLoopItem),
    legal_representatives: legalReps.map(toPartyLoopItem),
    guardians: guardians.map(toPartyLoopItem),
    curators: curators.map(toPartyLoopItem),
    sellers: sellerList.map(toPartyLoopItem),
    landlords: landlordList.map(toPartyLoopItem),
    signers: signerList,

    owner: roleObject(primaryOwner),
    guarantor: roleObject(guarantors.find((p) => p.is_primary) || guarantors[0]),
    witness: roleObject(witnesses.find((p) => p.is_primary) || witnesses[0]),
    spouse: roleObject(spouses.find((p) => p.is_primary) || spouses[0]),
    attorney: roleObject(attorneys.find((p) => p.is_primary) || attorneys[0]),
    legal_representative: roleObject(legalReps.find((p) => p.is_primary) || legalReps[0]),
    guardian: roleObject(guardians.find((p) => p.is_primary) || guardians[0]),
    curator: roleObject(curators.find((p) => p.is_primary) || curators[0]),

    ...tenantsLegacy,
    ...buyersLegacy,
    ...ownersLegacy,
    ...guarantorsLegacy,
    ...witnessesLegacy,
    ...spousesLegacy,
    ...attorneysLegacy,
    ...legalRepsLegacy,
    ...guardiansLegacy,
    ...curatorsLegacy,
    ...sellersLegacy,
    ...landlordsLegacy,
    ...scalarQualifications("buyer", buyerObj),
    ...scalarQualifications("seller", sellerObj),
    ...scalarQualifications("tenant", tenantObj),
    ...scalarQualifications("landlord", landlordObj),
    // Canonical singular aliases follow the selected principal; indexed slots
    // keep their original array order for compatibility with existing models.
    buyer_name: buyerObj.name, buyer_email: buyerObj.email, buyer_phone: buyerObj.phone, buyer_document: buyerObj.document,
    tenant_name: tenantObj.name, tenant_email: tenantObj.email, tenant_phone: tenantObj.phone, tenant_document: tenantObj.document,


    property_id: property?.id || "",
    property_description: property?.description || "",
    property_status: property?.status || "",
    property_condition: property?.condition || "",
    property_country: property?.address?.country || "",
    property_state_code: property?.address?.state_code || property?.address?.stateCode || "",
    property_postal_code: property?.address?.postal_code || property?.address?.postalCode || "",
    property_total_area: property?.areas?.total_area_m2 !== undefined ? String(property.areas.total_area_m2) : "",
    property_usable_area: property?.areas?.usable_area_m2 !== undefined ? String(property.areas.usable_area_m2) : "",
    property_land_area: property?.areas?.land_area_m2 !== undefined ? String(property.areas.land_area_m2) : "",
    property_built_area: property?.areas?.built_area_m2 !== undefined ? String(property.areas.built_area_m2) : "",
    property_bedrooms: property?.features?.bedrooms !== undefined ? String(property.features.bedrooms) : "",
    property_bathrooms: property?.features?.bathrooms !== undefined ? String(property.features.bathrooms) : "",
    property_suites: property?.features?.suites !== undefined ? String(property.features.suites) : "",
    property_parking_spaces: property?.features?.parking_spaces !== undefined ? String(property.features.parking_spaces) : "",
    property_amenities_list: property?.amenities?.join("; ") || "",
    property_capture_exclusivity: property?.capture_exclusivity || "",
    property_intermediation_contract_id: property?.intermediation_contract_id || "",
    property_title: property?.title || "",
    property_display_id: property?.display_id || "",
    property_address: formattedAddress,
    property_street: property?.address?.street || "",
    property_number: property?.address?.number || "",
    property_complement: property?.address?.complement || "",
    property_neighborhood: property?.address?.neighborhood || "",
    property_city: property?.address?.city || "",
    property_state: property?.address?.state || "",
    property_zip: (property?.address as any)?.postalCode || (property?.address as any)?.postal_code || "",
    property_registration: PLACEHOLDER,
    property_area: property?.areas?.total_area_m2?.toString() || "",
    property_rooms: property?.features?.bedrooms?.toString() || "",
    property_type: propertyTypeLabel,
    property_listing_type: listingTypeLabel,
    property_sale_price: propertySalePrice !== null ? formatCurrencyBRL(propertySalePrice) : PLACEHOLDER,
    property_rent_price: propertyRentPrice !== null ? formatCurrencyBRL(propertyRentPrice) : PLACEHOLDER,
    property_condominium_fee: propertyCondoFee !== null ? formatCurrencyBRL(propertyCondoFee) : PLACEHOLDER,
    property_property_tax: propertyPropertyTax !== null ? formatCurrencyBRL(propertyPropertyTax) : PLACEHOLDER,
    property_owners_count: String(propertyOwnerIds.length),
    property_owner_ids: propertyOwnerIds.join(", ") || PLACEHOLDER,
    property_owner_names: propertyOwnerNames,
    property_capturing_brokers_count: String(capturing.length),
    property_capturing_brokers_summary: capturingSummary,

    deal_value: formatCurrencyBRL(dealValue),
    deal_value_written: currencyToWordsBRL(dealValue),
    down_payment: formatCurrencyBRL(downPayment),
    installments: String(installments),
    installment_value: formatCurrencyBRL(installmentValue),
    financed_value: formatCurrencyBRL(financedValue),
    financed_value_written: currencyToWordsBRL(financedValue),
    commission_value: formatCurrencyBRL(commissionValue),
    commission_pct: String(commissionPct),
    payment_method: proposal?.payment_method || "À Vista",

    deal_property_id: dealDetails.property_id || "",
    deal_customer_id: dealDetails.customer_id || "",
    deal_owner_id: dealDetails.owner_id || "",
    deal_created_at: dateBR(dealDetails.created_at),
    deal_updated_at: dateBR(dealDetails.updated_at),
    proposal_id: proposal?.id || "",
    proposal_status: proposal?.status || "",
    proposal_counter_value: proposal?.counter_value !== undefined ? formatCurrencyBRL(proposal.counter_value) : "",
    proposal_notes: proposal?.notes || "",
    proposal_created_at: dateBR(proposal?.created_at),
    broker_primary_registration: property?.capturing_brokers?.find((item) => item.id === dealDetails.broker_id)?.creci || (property?.broker?.id === dealDetails.broker_id ? property?.broker?.creci : "") || "",
    ...brokerRoleAliases,
    deal_id: dealDetails.id,
    deal_display_id: dealDetails.display_id || "",
    deal_type: dealDetails.type,
    deal_type_label: dealDetails.type === "SALE" ? "Venda" : "Locação",
    deal_status: dealDetails.status,
    proposal_value: formatCurrencyBRL(proposal?.value || dealValue),
    proposal_payment_method: proposal?.payment_method || "",
    proposal_installments: proposal?.installments ? String(proposal.installments) : "",
    proposal_down_payment: formatCurrencyBRL(proposal?.down_payment || 0),
    signers_count: String(signers.length),
    signers_list: signers.map((s) => `${s.name} (${s.role})`).join("; ") || PLACEHOLDER,
    broker_primary_id: dealDetails.broker_id || PLACEHOLDER,
    broker_primary_name: primaryBroker?.name || PLACEHOLDER,
    broker_primary_email: primaryBroker?.email || PLACEHOLDER,
    broker_participations_count: String(participations.length),
    broker_participations_summary: brokerParticipationsSummary,
    broker_captador_name: captador ? brokersById[captador.broker_id]?.name || captador.broker_id : PLACEHOLDER,
    broker_visitador_name: visitador ? brokersById[visitador.broker_id]?.name || visitador.broker_id : PLACEHOLDER,
    broker_fechamento_name: fechamento ? brokersById[fechamento.broker_id]?.name || fechamento.broker_id : PLACEHOLDER,
    due_diligence_status: dd?.status || PLACEHOLDER,
    due_diligence_score_serasa: typeof dd?.score_serasa === "number" ? String(dd.score_serasa) : PLACEHOLDER,
    due_diligence_cnd_status: dd?.cnd_status || PLACEHOLDER,
    due_diligence_legal_opinion: dd?.legal_opinion || PLACEHOLDER,
    due_diligence_updated_at: dd?.updated_at ? dateBR(dd.updated_at) : PLACEHOLDER,
    checklist_total: String(checklist.length),
    checklist_pending: String(checklistPending),
    checklist_approved: String(checklistApproved),
    checklist_rejected: String(checklistRejected),
    checklist_pending_required: String(checklistPendingRequired),

    rent_monthly_amount: isRentalDeal ? formatCurrencyBRL(dealValue) : PLACEHOLDER,
    rent_monthly_amount_written: isRentalDeal ? currencyToWordsBRL(dealValue) : PLACEHOLDER,
    rent_default_due_day: isRentalDeal ? String(defaultDueDay) : PLACEHOLDER,
    rent_first_charge_date: isRentalDeal ? dateBR(firstChargeIso) : PLACEHOLDER,
    rent_first_charge_type: isRentalDeal ? firstChargeType : PLACEHOLDER,
    rent_first_charge_type_label: isRentalDeal ? firstChargeTypeLabel : PLACEHOLDER,
    rent_standard_due_date: isRentalDeal ? dateBR(standardDueIso) : PLACEHOLDER,

    company_name: company?.name || PLACEHOLDER,
    company_legal_name: company?.legal_name || company?.name || PLACEHOLDER,
    company_document: company?.tax_id || PLACEHOLDER,
    company_email: company?.email || PLACEHOLDER,
    company_phone: company?.phone || PLACEHOLDER,
    company_address: companyAddress || PLACEHOLDER,

    contract_type: contract?.type || "",
    contract_value_written: contract?.value !== undefined ? currencyToWordsBRL(contract.value) : "",
    contract_late_fee_pct: contract?.late_fee_pct !== undefined ? String(contract.late_fee_pct) : "",
    contract_monthly_interest_pct: contract?.monthly_interest_pct !== undefined ? String(contract.monthly_interest_pct) : "",
    contract_security_deposit: contract?.security_deposit !== undefined ? formatCurrencyBRL(contract.security_deposit) : "",
    contract_security_deposit_written: contract?.security_deposit !== undefined ? currencyToWordsBRL(contract.security_deposit) : "",
    contract_created_at: dateBR(contract?.created_at),
    contract_updated_at: dateBR(contract?.updated_at),
    contract_signatories_count: contract ? String(contract.signatories?.length || 0) : "",
    contract_signature_provider: contract?.signature_provider || "",
    contract_id: contract?.id || PLACEHOLDER,
    contract_display_id: contract?.display_id || PLACEHOLDER,
    contract_status: contract?.status || PLACEHOLDER,
    contract_value: contract?.value !== undefined ? formatCurrencyBRL(contract.value) : PLACEHOLDER,
    contract_commission_pct: contract?.commission_pct !== undefined ? String(contract.commission_pct) : PLACEHOLDER,
    contract_adjust_index: contract?.adjust_index || contract?.index || PLACEHOLDER,
    contract_auto_adjust: (contract?.auto_adjust ?? contract?.autoIndex) !== undefined ? ((contract?.auto_adjust ?? contract?.autoIndex) ? "Sim" : "Não") : PLACEHOLDER,
    contract_next_adjustment: dateBR(contract?.next_adjustment || contract?.nextAdjustment),
    contract_start_date: dateBR(contractStartIso),
    contract_end_date: contractEndIso ? dateBR(contractEndIso) : PLACEHOLDER,
    contract_start_date_iso: contractStartIso,
    contract_end_date_iso: contractEndIso || PLACEHOLDER,
    contract_term_months: contractTermMonths,

    current_date: dateBR(currentDateIso),
    current_date_iso: currentDateIso,
    current_day: String(now.getDate()),
    current_month: capitalizeFirst(now.toLocaleString("pt-BR", { month: "long" })),
    current_month_name: capitalizeFirst(now.toLocaleString("pt-BR", { month: "long" })),
    current_month_number: String(now.getMonth() + 1).padStart(2, "0"),
    current_year: String(now.getFullYear()),
    city_date: property?.address?.city ? `${property.address.city}, ${dateBR(currentDateIso)}` : dateBR(currentDateIso),
  };
  for (const baseKey of CONTRACT_DATE_BASE_KEYS) {
    for (const format of ["iso", "written"] as const) {
      data[`${baseKey}_${format}`] = resolveContractDateVariable(data, `${baseKey}_${format}`) || "";
    }
  }
  return data;
}
