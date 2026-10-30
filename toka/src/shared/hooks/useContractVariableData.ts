import { useEffect, useMemo, useState } from "react";

import type { Contract } from "../contracts/types";
import type { ApiLead } from "../services/lead.service";
import type { BrokerDTO } from "../services/broker.service";
import type { DealDTO } from "../services/deal.service";
import type { PropertyDTO } from "../services/property.service";
import { dealService } from "../services/deal.service";
import { propertyService } from "../services/property.service";
import { leadService } from "../services/lead.service";
import { brokerService } from "../services/broker.service";
import { tenantFinancialSettingsService } from "../services/tenantFinancialSettings.service";
import { buildContractVariableData } from "../utils/contractVariableData";

/**
 * Monta os dados de variáveis do contrato a partir do próprio contrato.
 *
 * O editor da esteira digital já montava isso, mas a partir de estado que ele
 * carrega por conta própria (deal aberto, imóvel, leads das partes, corretores).
 * O editor de aditivo está numa tela de contrato, sem esse contexto — daí este
 * hook: parte dos IDs do contrato e carrega o mesmo conjunto.
 *
 * O objetivo é reusar `buildContractVariableData` (a mesma função da esteira) em
 * vez de recriar a montagem. Sem isso os dois editores divergiriam com o tempo,
 * que é exatamente o que o requisito de consistência pede para evitar.
 */
/**
 * Data em `YYYY-MM-DD`.
 *
 * O builder de variáveis monta `new Date(`${valor}T00:00:00Z`).toISOString()`
 * para contrato de locação; com valor vazio isso é `Invalid time value` e
 * derruba a tela inteira. Datas ausentes viram string vazia de forma explícita,
 * e quem consome já trata ausência.
 */
function dateOnly(value?: string | null): string {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "";
  return parsed.toISOString().slice(0, 10);
}

export function useContractVariableData(contract: Contract | null) {
  const [deal, setDeal] = useState<DealDTO | null>(null);
  const [property, setProperty] = useState<PropertyDTO | null>(null);
  const [buyerLead, setBuyerLead] = useState<ApiLead | null>(null);
  const [sellerLead, setSellerLead] = useState<ApiLead | null>(null);
  const [brokersById, setBrokersById] = useState<Record<string, BrokerDTO>>({});
  const [ownerLeadsById, setOwnerLeadsById] = useState<Record<string, ApiLead>>({});
  // Dia de vencimento padrão vem das configurações da imobiliária, não de um
  // número fixo no código: é o mesmo valor que o financeiro usa ao gerar
  // cobranças e que o editor da esteira usa nas variáveis do contrato.
  const [defaultDueDay, setDefaultDueDay] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    // Falha aqui não pode impedir o preenchimento das outras variáveis: sem a
    // configuração o campo fica vazio, que é melhor do que um dia inventado.
    tenantFinancialSettingsService
      .get()
      .then((settings) => {
        const configured = settings?.default_due_day;
        if (cancelled || !configured) return;
        // Mesmo limite do editor da esteira, para os dois não divergirem.
        setDefaultDueDay(Math.min(28, Math.max(1, configured)));
      })
      .catch(() => {
        if (!cancelled) setDefaultDueDay(null);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    if (!contract?.id) {
      setDeal(null);
      setProperty(null);
      setBuyerLead(null);
      setSellerLead(null);
      return () => {
        cancelled = true;
      };
    }

    setLoading(true);
    (async () => {
      try {
        if (contract.deal_id) {
          const response = await dealService.getById(contract.deal_id);
          const loadedDeal = (response as { data?: { data?: DealDTO } })?.data?.data ?? null;
          if (!cancelled) setDeal(loadedDeal);

          // Os leads são referenciados por `lead_id`/`customer_id` do negócio.
          // Usar o id da PARTE aqui buscava um lead inexistente (404) e deixava
          // as variáveis de comprador/vendedor sem preenchimento.
          const customerLeadId = loadedDeal?.lead_id || loadedDeal?.customer_id;
          if (customerLeadId) {
            const lead = await leadService.getById(customerLeadId).catch(() => null);
            if (!cancelled && lead) setBuyerLead(lead);
          }
          const ownerLeadId = (loadedDeal as { owner_id?: string } | null)?.owner_id;
          if (ownerLeadId) {
            const lead = await leadService.getById(ownerLeadId).catch(() => null);
            if (!cancelled && lead) {
              setSellerLead(lead);
              setOwnerLeadsById((current) => ({ ...current, [ownerLeadId]: lead }));
            }
          }

          const brokerIds = new Set<string>();
          if (loadedDeal?.broker_id) brokerIds.add(loadedDeal.broker_id);
          for (const participation of loadedDeal?.broker_participations ?? []) {
            if (participation?.broker_id) brokerIds.add(participation.broker_id);
          }
          const loadedBrokers: Record<string, BrokerDTO> = {};
          await Promise.all(
            Array.from(brokerIds).map(async (brokerId) => {
              const response = await brokerService.getById(brokerId).catch(() => null);
              const broker = response?.data?.data;
              if (broker) loadedBrokers[brokerId] = broker;
            }),
          );
          if (!cancelled) setBrokersById(loadedBrokers);
        }

        const propertyId = contract.property_id || deal?.property_id;
        if (propertyId) {
          const response = await propertyService.getById(propertyId).catch(() => null);
          const loadedProperty = response?.data?.data;
          if (!cancelled && loadedProperty) setProperty(loadedProperty);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
    };
    // `deal` não entra: usar o carregado aqui causaria laço de recarga.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [contract?.id, contract?.deal_id, contract?.property_id]);

  const data = useMemo(
    () =>
      buildContractVariableData({
        dealDetails: deal,
        property,
        signers: [],
        buyerLead,
        sellerLead,
        brokersById,
        ownerLeadsById,
        rentalStartDate: dateOnly(contract?.start_date),
        rentalEndDate: dateOnly(contract?.end_date),
        computedEndDate: dateOnly(contract?.end_date),
        firstChargeDate: dateOnly(contract?.start_date),
        firstChargeType: "same_month_pro_rata",
        // 0 é ausência de configuração, e o builder trata esse número como dia
        // do mês — `setUTCDate(0)` cairia no último dia do mês ANTERIOR, gerando
        // uma data de vencimento plausível e errada. Deterministicamente
        // inválido evita a data inventada; o rótulo do dia fica vazio.
        defaultDueDay: defaultDueDay ?? Number.NaN,
        contract,
      }),
    [deal, property, buyerLead, sellerLead, brokersById, ownerLeadsById, contract, defaultDueDay],
  );

  return { data, loading, deal, property };
}
