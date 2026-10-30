import type { ReactNode } from 'react';
import { clsx } from 'clsx';

/**
 * Cabeçalho padrão dos painéis laterais de inadimplência: título, identificação
 * do caso (contrato + devedor do SERVIDOR) e os botões de expandir/fechar que o
 * `ExpandableSidePanel` injeta.
 */
export function DelinquencyPanelHeader({
  title,
  icon,
  contractId,
  debtorName,
  expandButton,
  closeButton,
  destructive,
}: {
  title: string;
  icon: ReactNode;
  contractId?: string | null;
  debtorName?: string | null;
  expandButton?: ReactNode;
  closeButton: ReactNode;
  destructive?: boolean;
}) {
  return (
    <div className="px-6 py-4 border-b border-border flex items-start justify-between gap-3 bg-card">
      <div className="flex min-w-0 items-center gap-3">
        <div
          className={clsx(
            'w-10 h-10 shrink-0 rounded-full flex items-center justify-center',
            destructive ? 'bg-status-critical-soft text-status-critical' : 'bg-primary-50 text-primary-600',
          )}
        >
          {icon}
        </div>
        <div className="min-w-0">
          <h2 className="text-xl font-bold text-foreground truncate">{title}</h2>
          <p className="text-xs text-muted-foreground mt-0.5 truncate">
            {contractId ? `Contrato ${contractId}` : 'Contrato —'}
            {debtorName ? ` · ${debtorName}` : ''}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {expandButton}
        {closeButton}
      </div>
    </div>
  );
}
