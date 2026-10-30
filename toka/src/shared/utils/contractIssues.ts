import type { NavigateFunction } from "react-router";
import { toast } from "sonner";

export type ContractIssueRef = "property" | "owner" | "tenant";

export interface ContractIssue {
  ref: ContractIssueRef;
  message: string;
  ctaLabel: string;
  onCtaClick: () => void;
}

export type ContractIssueContract = {
  id: string;
  property_id?: string | null;
  customer_id?: string | null;
};

export type ContractIssueProperty = {
  ownerId?: string | null;
  owners?: Array<{ id: string }> | null;
};

export function buildContractIssues(
  contract: ContractIssueContract,
  property: ContractIssueProperty | undefined,
  navigate: NavigateFunction
): ContractIssue[] {
  const propertyId = contract.property_id ?? "";
  const ownerId = property?.ownerId || property?.owners?.[0]?.id || "";
  const tenantId = contract.customer_id ?? "";

  const issues: ContractIssue[] = [];

  if (!propertyId) {
    issues.push({
      ref: "property",
      message: "Contrato sem imóvel vinculado",
      ctaLabel: "Abrir contrato",
      onCtaClick: () => navigate(`/contratos/${contract.id}?focus=property`),
    });
  } else if (!ownerId) {
    issues.push({
      ref: "owner",
      message: "Imóvel sem proprietário cadastrado",
      ctaLabel: "Abrir imóvel",
      onCtaClick: () => navigate(`/imoveis/${propertyId}?focus=owner`),
    });
  }
  if (!tenantId) {
    issues.push({
      ref: "tenant",
      message: "Contrato sem inquilino vinculado",
      ctaLabel: "Abrir contrato",
      onCtaClick: () => navigate(`/contratos/${contract.id}?focus=tenant`),
    });
  }

  return issues;
}

export function showContractIssueToasts(issues: ContractIssue[]): void {
  issues.forEach((issue) => {
    toast.error(issue.message, {
      action: {
        label: issue.ctaLabel,
        onClick: issue.onCtaClick,
      },
    });
  });
}

