import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { logger } from "../../../../lib/logger";
import type { Contract, SignatureEnvelope, SignatureEnvelopeSigner } from "../../../../shared/contracts/types";
import type { Signer } from "../../../../shared/services/deal.service";
import { dealService } from "../../../../shared/services/deal.service";
import { contractService } from "../../../../shared/services/contract.service";
import { signatureService } from "../../../../shared/services/signature.service";
import type { SignerFormValues } from "../../../../shared/components/signature/SignerFormModal";
import { useWsSubscription } from "../../../../shared/ws/useWsSubscription";

const ENVELOPE_FLOW_STATUSES = ["DRAFT", "PENDING_SIGNATURE", "PARTIALLY_SIGNED"];

interface UseContractSignatureEnvelopeArgs {
  contract: Contract | null;
  dealId?: string;
  dealSigners: Signer[];
  onContractChange: (contract: Contract) => void;
  onRefresh: () => void;
}

export interface ContractSignatureEnvelopeController {
  envelope: SignatureEnvelope | null;
  envelopeLoading: boolean;
  envelopeLoadFailed: boolean;
  reconcileBusy: boolean;
  cancelBusy: boolean;
  cancelConfirmOpen: boolean;
  resendAllBusy: boolean;
  resendingSignerId: string | null;
  signerEditing: SignatureEnvelopeSigner | null;
  envelopeAddOpen: boolean;
  handleReconcile: () => Promise<void>;
  handleCancel: () => Promise<void>;
  handleResendAll: () => Promise<void>;
  handleResendSigner: (signerId: string) => Promise<void>;
  handleEditSignerSubmit: (values: SignerFormValues) => Promise<void>;
  handleAddSignerToEnvelope: (values: SignerFormValues) => Promise<void>;
  setCancelConfirmOpen: (open: boolean) => void;
  setSignerEditing: (signer: SignatureEnvelopeSigner | null) => void;
  setEnvelopeAddOpen: (open: boolean) => void;
}

export function useContractSignatureEnvelope({
  contract,
  dealId,
  dealSigners,
  onContractChange,
  onRefresh,
}: UseContractSignatureEnvelopeArgs): ContractSignatureEnvelopeController {
  const [envelope, setEnvelope] = useState<SignatureEnvelope | null>(null);
  const [envelopeLoading, setEnvelopeLoading] = useState(false);
  const [envelopeLoadFailed, setEnvelopeLoadFailed] = useState(false);
  const [reconcileBusy, setReconcileBusy] = useState(false);
  const [cancelBusy, setCancelBusy] = useState(false);
  const [cancelConfirmOpen, setCancelConfirmOpen] = useState(false);
  const [resendAllBusy, setResendAllBusy] = useState(false);
  const [resendingSignerId, setResendingSignerId] = useState<string | null>(null);
  const [signerEditing, setSignerEditing] = useState<SignatureEnvelopeSigner | null>(null);
  const [envelopeAddOpen, setEnvelopeAddOpen] = useState(false);

  const dealHasPendingSigners = useMemo(() => {
    if (dealSigners.length === 0) return false;
    return dealSigners.some(
      (signer) => signer.status && signer.status !== "SIGNED" && signer.status !== "REJECTED",
    );
  }, [dealSigners]);

  const shouldLoadEnvelope = useMemo(() => {
    if (!contract?.id) return false;
    if (ENVELOPE_FLOW_STATUSES.includes(contract.status as string)) return true;
    return contract.status === "SIGNED" && dealHasPendingSigners;
  }, [contract?.id, contract?.status, dealHasPendingSigners]);

  useEffect(() => {
    let active = true;
    setEnvelope(null);
    setEnvelopeLoadFailed(false);
    if (!shouldLoadEnvelope) {
      setEnvelopeLoading(false);
      return;
    }
    setEnvelopeLoading(true);
    signatureService
      .getByContract(contract!.id)
      .then((next) => { if (active) setEnvelope(next); })
      .catch(() => { if (active) setEnvelopeLoadFailed(true); })
      .finally(() => { if (active) setEnvelopeLoading(false); });
    return () => { active = false; };
  }, [shouldLoadEnvelope, contract?.id, contract?.updated_at]);

  const handleReconcile = async () => {
    if (!envelope?.id || reconcileBusy) return;
    setReconcileBusy(true);
    try {
      const updated = await signatureService.reconcile(envelope.id);
      setEnvelope(updated);

      if (dealId) {
        try {
          await dealService.reconcileSigners(dealId);
        } catch (dealError) {
          logger.error("deal-side reconcile failed", dealError);
        }
      }

      toast.success("Status sincronizado.");
      onRefresh();
    } catch (error) {
      logger.error("reconcile failed", error);
      toast.error("Não foi possível sincronizar agora. Tente novamente em instantes.");
    } finally {
      setReconcileBusy(false);
    }
  };

  const handleCancel = async () => {
    if (!envelope?.id || cancelBusy) return;
    setCancelBusy(true);
    try {
      const updated = await signatureService.cancel(envelope.id);
      setEnvelope(updated);
      toast.success("Assinatura cancelada.");
      setCancelConfirmOpen(false);
      onRefresh();
    } catch (error) {
      logger.error("cancel envelope failed", error);
      // A mensagem do servidor aqui importa: quando o provedor recusa o
      // cancelamento (502) ele diz que o link de assinatura CONTINUA válido —
      // o operador precisa saber disso para não achar que já cancelou. Antes
      // esta tela mostrava só "tente novamente em instantes" e a falha do
      // provedor ficava invisível (incidente de 2026-10-05).
      const apiMessage = (error as { response?: { data?: { message?: string } } })?.response?.data?.message;
      toast.error(apiMessage ?? "Não foi possível cancelar agora. Tente novamente em instantes.");
    } finally {
      setCancelBusy(false);
    }
  };

  const handleResendAll = async () => {
    if (!envelope?.id || resendAllBusy) return;
    setResendAllBusy(true);
    try {
      await signatureService.resend(envelope.id);
      toast.success("Notificações reenviadas para todos os pendentes.");
    } catch (error) {
      logger.error("resend envelope failed", error);
      toast.error("Não foi possível reenviar agora. Tente novamente em instantes.");
    } finally {
      setResendAllBusy(false);
    }
  };

  const handleResendSigner = async (signerId: string) => {
    if (!envelope?.id || resendingSignerId) return;
    setResendingSignerId(signerId);
    try {
      await signatureService.resend(envelope.id, signerId);
      toast.success("Notificação reenviada.");
    } catch (error) {
      logger.error("resend signer failed", error);
      toast.error("Não foi possível reenviar para este assinante.");
    } finally {
      setResendingSignerId(null);
    }
  };

  const handleEditSignerSubmit = async (values: SignerFormValues) => {
    if (!envelope?.id || !signerEditing) return;
    const updated = await signatureService.editSigner(envelope.id, signerEditing.signer_id, {
      name: values.name,
      email: values.email,
      phone_number: values.phone_number,
      documentation: values.documentation,
      birthday: values.birthday,
    });
    setEnvelope(updated);
    toast.success("Signatário atualizado.");
    onRefresh();
  };

  const handleAddSignerToEnvelope = async (values: SignerFormValues) => {
    if (!envelope?.id) return;
    const updated = await signatureService.addSigner(envelope.id, {
      name: values.name,
      email: values.email,
      phone_number: values.phone_number,
      documentation: values.documentation,
      birthday: values.birthday,
      role: values.role,
      auth: values.auth,
    });
    setEnvelope(updated);
    toast.success("Signatário adicionado ao envelope.");
    onRefresh();
  };

  useWsSubscription("signature_update", (event) => {
    if (!contract?.id || event.payload?.contract_id !== contract.id) return;

    if (dealId) {
      contractService
        .listByDeal(dealId)
        .then((contracts) => {
          if (contracts?.length) onContractChange(contracts[0]);
        })
        .catch(() => {
          // Keep the last known contract when realtime reconciliation fails.
        });
    }

    if (envelope?.id) {
      signatureService
        .getByContract(contract.id)
        .then(setEnvelope)
        .catch(() => {
          // Keep the last known envelope; the operator can reconcile manually.
        });
    }
    onRefresh();
  });

  return {
    envelope,
    envelopeLoading,
    envelopeLoadFailed,
    reconcileBusy,
    cancelBusy,
    cancelConfirmOpen,
    resendAllBusy,
    resendingSignerId,
    signerEditing,
    envelopeAddOpen,
    handleReconcile,
    handleCancel,
    handleResendAll,
    handleResendSigner,
    handleEditSignerSubmit,
    handleAddSignerToEnvelope,
    setCancelConfirmOpen,
    setSignerEditing,
    setEnvelopeAddOpen,
  };
}
