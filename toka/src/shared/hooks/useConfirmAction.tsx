import { useCallback, useRef, useState } from "react";
import { ConfirmDialog } from "../components/ConfirmDialog";

type Confirmation = {
  title?: string;
  confirmLabel?: string;
  variant?: "danger" | "warning" | "default";
};

/** Capture the operation at the time it was requested, and keep it open on error. */
export function useConfirmAction() {
  const active = useRef(false);
  const [request, setRequest] = useState<{
    description: string;
    action: () => unknown | Promise<unknown>;
    options: Confirmation;
  } | null>(null);
  const confirmAction = useCallback((description: string, action: () => unknown | Promise<unknown>, options: Confirmation = {}) => {
    if (active.current) return;
    active.current = true;
    setRequest({ description, action, options });
  }, []);
  const close = () => { active.current = false; setRequest(null); };
  const confirmationDialog = request ? (
    <ConfirmDialog
      open
      title={request.options.title ?? "Confirmar ação"}
      description={request.description}
      confirmLabel={request.options.confirmLabel ?? "Confirmar"}
      variant={request.options.variant ?? "warning"}
      onCancel={close}
      onConfirm={async () => { await request.action(); close(); }}
    />
  ) : null;
  return { confirmAction, confirmationDialog };
}
