export type DealStage =
  | "Em Negociação"
  | "Proposta"
  | "Análise"
  | "Contrato"
  | "Concluído"
  | "Perdido";

export interface BrokerParticipation {
  broker_id: string;
  role: string;
  commission?: number;
  percentage?: number;
}

export interface EsteiraDeal {
  id: string;
  displayId?: string;
  client: string;
  client_email?: string;
  client_phone?: string;
  client_doc?: string;
  clientId?: string;
  ownerId?: string;
  property: string;
  propertyId?: string;
  type: "Venda" | "Locação";
  value: number;
  /** Percentual de comissão configurado na esteira (ex: 6 para 6%) */
  commission?: number;
  /** Participações de corretores com split de comissão */
  brokerParticipations?: BrokerParticipation[];
  stage: DealStage;
  date: string;
  created_at?: string;
  updated_at?: string;
  alerts?: string[];
  avatarUrl: string;
  brokerId?: string;
  hasCounterOffer?: boolean;
  latestCounterValue?: number;
  latestCounterProposalId?: string;
}
