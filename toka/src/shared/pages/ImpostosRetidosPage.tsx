import { Stamp } from "lucide-react";

import { ImpostosRetidosTab } from "./Finance/ImpostosRetidosTab";
import { FinanceAreaNav } from "../components/finance/FinanceAreaNav";

/**
 * Página de "Impostos retidos" — o passivo da imobiliária com os tributos
 * retidos na comissão do corretor (IRRF/INSS/ISS/Outros), com competência,
 * vencimento, comprovante de recolhimento e baixa.
 *
 * Como as demais áreas canônicas do financeiro, vive em rota própria
 * (`/financeiro/impostos`) com a navegação compartilhada (`FinanceAreaNav`) e
 * busca os próprios dados via `taxObligationService` — a obrigação é derivada
 * do desconto lançado na comissão, então não há formulário de criação aqui.
 */
export default function ImpostosRetidosPage() {
  return (
    <div className="mx-auto max-w-7xl space-y-4 p-5 lg:p-6">
      <FinanceAreaNav />
      <div>
        <h1 className="flex items-center gap-2 text-2xl font-bold text-foreground">
          <Stamp className="h-6 w-6 text-primary-600" />
          Impostos retidos
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          IRRF, INSS e ISS retidos da comissão do corretor: quanto há a recolher, o que está vencido e a baixa com
          comprovante da guia.
        </p>
      </div>
      <ImpostosRetidosTab />
    </div>
  );
}
