import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router';
import { Button } from '../../../shared/ui/button';
import type { DealDTO } from '../../../shared/services/deal.service';
import { portalService, type PortalOverviewLead } from '../services/portal.service';
import { usePortalContact, type PortalContactTarget } from '../hooks/usePortalContact';

export function PortalContactChooser({ onOpened }: { onOpened?: () => void }) {
  const [params] = useSearchParams();
  const [expanded, setExpanded] = useState(params.get('contact') === '1');
  const [leads, setLeads] = useState<PortalOverviewLead[]>([]);
  const [deals, setDeals] = useState<DealDTO[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const { openContact, opening } = usePortalContact();
  const chooseContact = async (target: PortalContactTarget) => {
    if (await openContact(target)) {
      setExpanded(false);
      onOpened?.();
    }
  };
  useEffect(() => { setExpanded(params.get('contact') === '1'); }, [params]);
  useEffect(() => {
    if (!expanded) return;
    let cancelled = false;
    setLoading(true);
    setError(false);
    Promise.allSettled([portalService.getOverview(), portalService.getDeals()]).then(([leadResult, dealResult]) => {
      if (cancelled) return;
      setLeads(leadResult.status === 'fulfilled' && Array.isArray(leadResult.value) ? leadResult.value : []);
      setDeals(dealResult.status === 'fulfilled' && Array.isArray(dealResult.value) ? dealResult.value as DealDTO[] : []);
      setError(leadResult.status === 'rejected' || dealResult.status === 'rejected');
      setLoading(false);
    });
    return () => { cancelled = true; };
  }, [expanded, attempt]);
  return <div className="border-b border-border p-4 space-y-3">
    <Button variant="outline" className="w-full" onClick={() => setExpanded(!expanded)} aria-expanded={expanded}>Falar com a imobiliária</Button>
    {expanded && <div className="space-y-2">
      <p className="text-sm text-muted-foreground">Escolha o atendimento para abrir a conversa.</p>
      {loading && <p role="status" className="text-sm">Carregando atendimentos…</p>}
      {error && <div role="alert" className="text-sm">Não foi possível carregar todos os atendimentos. <Button variant="ghost" onClick={() => setAttempt(attempt + 1)}>Tentar novamente</Button></div>}
      {!loading && !error && !leads.length && !deals.length && <p className="text-sm text-muted-foreground">Nenhum atendimento vinculado à sua conta. As conversas já iniciadas pela equipe continuam na lista abaixo.</p>}
      {leads.map((lead) => <Button key={lead.id} variant="outline" disabled={opening} className="w-full h-auto whitespace-normal text-left justify-start" onClick={() => void chooseContact({ leadId: lead.id })}>
        {lead.broker_name || 'Equipe da imobiliária'} — {lead.recommended_property?.title || lead.name || 'Atendimento'}
      </Button>)}
      {deals.map((deal) => <Button key={deal.id} variant="outline" disabled={opening} className="w-full h-auto whitespace-normal text-left justify-start" onClick={() => void chooseContact({ dealId: deal.id })}>
        Conversa da negociação {deal.display_id || deal.id.slice(0, 8)}
      </Button>)}
    </div>}
  </div>;
}
