import type { ReactNode } from 'react';
import { AlertCircle, Info } from 'lucide-react';
import { clsx } from 'clsx';

/** Aviso dentro dos painéis — explica bloqueio de permissão ou regra do fluxo. */
export function PanelNotice({
  tone = 'info',
  children,
}: {
  tone?: 'info' | 'critical';
  children: ReactNode;
}) {
  return (
    <div
      role="status"
      className={clsx(
        'flex items-start gap-3 rounded-xl border p-4 text-sm',
        tone === 'critical'
          ? 'bg-status-critical-soft border-status-critical/30 text-status-critical'
          : 'bg-primary-50 border-primary-100 text-primary-800',
      )}
    >
      {tone === 'critical' ? <AlertCircle className="w-4 h-4 mt-0.5 shrink-0" /> : <Info className="w-4 h-4 mt-0.5 shrink-0" />}
      <div className="min-w-0 space-y-1">{children}</div>
    </div>
  );
}
