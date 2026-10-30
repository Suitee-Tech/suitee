import { usePortalContact } from "../hooks/usePortalContact";
import { Link } from "react-router";
import { ChevronRight, Info, MessageCircle } from "lucide-react";
import { Button } from "../../../shared/ui/button";
import { Skeleton } from "../../../shared/ui/skeleton";
import { DealSwitcher } from "../components/DealSwitcher";
import { usePortalDeals } from "../context/PortalDealsContext";
import { usePortalPersona } from "../hooks/usePortalPersona";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";

export function PortalSupport() {
  const { deals, activeDeal, activeDealId, isLoading, setActiveDealId } = usePortalDeals();
  // Suporte por persona: manutenção só é fluxo de locação e a resposta sobre
  // visita muda de lado — quem anuncia não "agenda visita" ao próprio imóvel.
  const { persona, copy } = usePortalPersona();
  const isRentJourney = persona.isRent || persona.actor === "tenant" || persona.actor === "landlord";
  const visitFaq = copy.visits.canSchedule
    ? `Vá até a aba "${copy.propertiesNavLabel}" ou abra o imóvel desejado e toque em "${copy.propertyVisitCta ?? "Solicitar visita"}". Você também pode combinar pelo chat com o corretor.`
    : "As visitas ao seu imóvel são agendadas pela imobiliária com os interessados. Acompanhe os horários em \"Sua jornada\" — você não precisa agendar nada.";

  const { openContact, opening } = usePortalContact();

  if (isLoading) {
    return (
      <PortalPage className="px-5">
        <PortalPageHeader title="Suporte" className="-mx-5" />
        <div className="flex flex-col gap-3">
          <Skeleton className="h-32 w-full rounded-3xl" />
          <Skeleton className="h-48 w-full rounded-3xl mt-4" />
        </div>
      </PortalPage>
    );
  }

  return (
    <PortalPage className="px-5">
      <PortalPageHeader title="Suporte" className="-mx-5" />

      {deals.length > 1 && (
        <div className="mb-5">
          <DealSwitcher
            deals={deals}
            activeDealId={activeDealId}
            onSelect={setActiveDealId}
            title="Selecionar transação (suporte)"
          />
        </div>
      )}

      <div className="bg-status-ok-soft border border-status-ok/20 rounded-3xl p-6 shadow-sm mb-6">
        <h2 className="text-[16px] font-bold text-status-ok mb-1">Fale com a imobiliária</h2>
        <p className="text-[14px] text-status-ok mb-6">Converse com a equipe responsável pelo seu atendimento.</p>
        <Button disabled={opening} onClick={() => void openContact({ dealId: activeDeal?.id })} className="w-full h-11 rounded-xl">
          <MessageCircle className="size-4 mr-2" /> {opening ? 'Abrindo conversa…' : 'Falar com o corretor'}
        </Button>
      </div>

      {isRentJourney && (
        <div className="bg-card border border-border rounded-3xl shadow-sm mb-6 overflow-hidden">
          <div className="p-5 border-b border-border/50">
            <h2 className="text-title font-bold text-foreground">{copy.ticketsTitle}</h2>
            <p className="text-label text-muted-foreground mt-1">
              {persona.actor === "landlord"
                ? "Registre e acompanhe reparos do imóvel que está alugado."
                : "Abra um chamado para reparos no imóvel que você aluga."}
            </p>
          </div>
          <div className="p-5 bg-muted/50">
            <Button asChild className="w-full h-11 rounded-xl text-[14px] font-bold">
              <Link to="/portal/chamados">
                Abrir Chamado <ChevronRight className="w-4 h-4 ml-1" />
              </Link>
            </Button>
          </div>
        </div>
      )}

      <div className="bg-card border border-border rounded-3xl p-5 shadow-sm">
        <h2 className="text-title font-bold text-foreground mb-4 flex items-center gap-2">
          <Info className="w-5 h-5 text-status-ok" />
          Dúvidas Frequentes
        </h2>

        <div className="divide-y divide-border">
          {!persona.isOwnerSide && (
            <div className="py-3">
              <p className="text-label font-bold text-foreground/80">Como acompanho a aprovação do meu crédito?</p>
              <p className="text-label text-muted-foreground mt-1 leading-relaxed">
                Você será notificado por e-mail e através do app na aba &quot;Notificações&quot; assim que houver qualquer atualização na
                análise de crédito.
              </p>
            </div>
          )}
          <div className="py-3">
            <p className="text-label font-bold text-foreground/80">Como funciona a assinatura do contrato?</p>
            <p className="text-label text-muted-foreground mt-1 leading-relaxed">
              O contrato será enviado para o seu e-mail através da nossa plataforma de assinatura digital. O processo é 100% online.
            </p>
          </div>
          <div className="py-3">
            <p className="text-label font-bold text-foreground/80">
              {copy.visits.canSchedule ? "Como solicito uma nova visita?" : "Como funcionam as visitas ao meu imóvel?"}
            </p>
            <p className="text-label text-muted-foreground mt-1 leading-relaxed">{visitFaq}</p>
          </div>
        </div>
      </div>
    </PortalPage>
  );
}
