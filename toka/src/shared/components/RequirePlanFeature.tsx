import type { ReactNode } from "react";
import { Crown, RefreshCw } from "lucide-react";
import { useNavigate } from "react-router";
import { Button } from "../ui/button";
import { useSubscriptionAccess } from "../context/SubscriptionAccessContext";

export function RequirePlanFeature({ feature, children }: { feature: string; children: ReactNode }) {
  const { access, isLoading, error, refresh, hasFeature } = useSubscriptionAccess();
  const navigate = useNavigate();

  if (isLoading && !access) {
    return <div className="flex min-h-[40vh] items-center justify-center text-sm text-muted-foreground" role="status">Consultando seu plano…</div>;
  }
  if (!access?.access_control_enabled || hasFeature(feature)) return <>{children}</>;

  const expired = !access.entitled;
  return (
    <section className="mx-auto my-12 max-w-2xl rounded-2xl border border-primary/25 bg-card p-8 text-center shadow-sm" aria-labelledby="plan-feature-title">
      <div className="mx-auto flex size-12 items-center justify-center rounded-full bg-primary/10 text-primary"><Crown className="size-6" /></div>
      <h1 id="plan-feature-title" className="mt-4 text-2xl font-semibold">
        {expired ? "Seu período de teste terminou" : "Recurso disponível no plano Pro"}
      </h1>
      <p className="mx-auto mt-2 max-w-lg text-sm text-muted-foreground">
        {expired
          ? "Escolha uma assinatura para continuar usando os recursos da plataforma. Seus dados continuam vinculados à sua conta."
          : "O plano Pro inclui relatórios, inteligência, assinatura digital e portal do cliente. Veja os planos e escolha o que acompanha sua operação."}
      </p>
      {error && <p className="mt-3 text-sm text-status-warning" role="status">{error}</p>}
      <div className="mt-6 flex flex-wrap justify-center gap-3">
        <Button onClick={() => navigate("/assinatura")}>
          <Crown className="mr-2 size-4" />{expired ? "Assinar para continuar" : "Conhecer o Pro"}
        </Button>
        {error && <Button variant="outline" onClick={() => void refresh()}><RefreshCw className="mr-2 size-4" />Tentar novamente</Button>}
      </div>
    </section>
  );
}
