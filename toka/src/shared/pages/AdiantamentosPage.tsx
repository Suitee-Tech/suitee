import { HandCoins } from "lucide-react";

import { AdiantamentosTab } from "./Finance/AdiantamentosTab";
import { FinanceAreaNav } from "../components/finance/FinanceAreaNav";

/**
 * Página de "Adiantamentos" — o dinheiro que a imobiliária adianta ao corretor
 * e recupera abatendo nas comissões seguintes.
 *
 * Como as demais áreas canônicas do financeiro, vive em rota própria
 * (`/financeiro/adiantamentos`) com a navegação compartilhada (`FinanceAreaNav`)
 * e busca os próprios dados via `brokerAdvanceService`. O abatimento não é
 * lançado aqui: é um desconto do tipo "Adiantamento" na comissão do corretor.
 */
export default function AdiantamentosPage() {
  return (
    <div className="mx-auto max-w-7xl space-y-4 p-5 lg:p-6">
      <FinanceAreaNav />
      <div>
        <h1 className="flex items-center gap-2 text-2xl font-bold text-foreground">
          <HandCoins className="h-6 w-6 text-primary-600" />
          Adiantamentos
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Adiantamentos feitos ao corretor, o saldo a recuperar e o que já foi abatido nas comissões seguintes.
        </p>
      </div>
      <AdiantamentosTab />
    </div>
  );
}
