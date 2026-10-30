import { useState } from 'react';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import { portalService } from '../services/portal.service';
import { notifyApiError } from "../../../shared/services/errorToast";

export interface PortalContactTarget { leadId?: string; dealId?: string }

/** Every contact entry opens a persisted conversation, or the contact chooser. */
export function usePortalContact() {
  const navigate = useNavigate();
  const [opening, setOpening] = useState(false);
  const openContact = async ({ leadId, dealId }: PortalContactTarget = {}) => {
    if (opening) return;
    if (!leadId && !dealId) {
      navigate('/portal/mensagens?contact=1');
      return;
    }
    setOpening(true);
    try {
      const channelId = leadId
        ? (await portalService.openDirectChannel(leadId)).channel_id
        : (await portalService.openDealChannel(dealId!)).id;
      if (!channelId) throw new Error('A conversa não foi disponibilizada. Tente novamente.');
      navigate(`/portal/mensagens?channel=${encodeURIComponent(channelId)}`);
      return channelId;
    } catch (error: any) {
      notifyApiError(error, { context: "Abrir conversa" });
    } finally {
      setOpening(false);
    }
  };
  return { openContact, opening };
}
