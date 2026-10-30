import { BellRing, CircleAlert, Volume2 } from "lucide-react";
import { Switch } from "../ui/switch";
import { Button } from "../ui/button";
import type { SystemNotificationPermission } from "../notifications/systemNotifications";

export interface SystemNotificationControlsProps {
  supported: boolean;
  permission: SystemNotificationPermission;
  enabled: boolean;
  sound: boolean;
  /** Ativa (pede permissão no gesto). Devolve a permissão final. */
  onEnable: () => Promise<SystemNotificationPermission>;
  onSetEnabled: (v: boolean) => void;
  onSetSound: (v: boolean) => void;
  /** Notificação de teste (com som conforme a preferência atual). */
  onTest: () => void;
}

/**
 * Controles de notificações de sistema no dropdown do sino. Apresentacional:
 * o estado vem do hook `useSystemNotifications` do componente pai.
 */
export function SystemNotificationControls(props: SystemNotificationControlsProps) {
  const { supported, permission, enabled, sound, onEnable, onSetEnabled, onSetSound, onTest } = props;

  if (!supported) {
    return (
      <div className="flex items-start gap-2 text-xs text-muted-foreground p-3 border-t border-border bg-muted/50 rounded-b-xl">
        <CircleAlert className="w-4 h-4 shrink-0 mt-0.5" />
        <span>Este navegador não suporta notificações de sistema.</span>
      </div>
    );
  }

  const handleEnabledChange = async (checked: boolean) => {
    if (!checked) {
      onSetEnabled(false);
      return;
    }
    // Gesto do usuário: pede permissão aqui. "denied" deixa o switch desligado
    // (o navegador não permite re-prompt — mostramos a orientação abaixo).
    await onEnable();
  };

  return (
    <div className="border-t border-border bg-muted/50 rounded-b-xl">
      <div className="flex items-center justify-between px-4 py-2.5">
        <div className="flex items-center gap-2 min-w-0">
          <BellRing className="w-4 h-4 text-muted-foreground shrink-0" />
          <div className="min-w-0">
            <p className="text-xs font-semibold text-foreground leading-tight">Notificações do sistema</p>
            <p className="text-micro text-muted-foreground leading-tight">Alertas do Chrome com a aba em segundo plano</p>
          </div>
        </div>
        <Switch
          checked={enabled && permission === "granted"}
          onCheckedChange={(checked) => void handleEnabledChange(checked)}
          aria-label="Notificações do sistema"
        />
      </div>

      <div className="flex items-center justify-between px-4 py-2.5">
        <div className="flex items-center gap-2 min-w-0">
          <Volume2 className="w-4 h-4 text-muted-foreground shrink-0" />
          <div className="min-w-0">
            <p className="text-xs font-semibold text-foreground leading-tight">Som</p>
            <p className="text-micro text-muted-foreground leading-tight">Som do sistema + alerta sonoro no app</p>
          </div>
        </div>
        <Switch checked={sound} onCheckedChange={onSetSound} aria-label="Som" />
      </div>

      {permission === "denied" && (
        <div className="flex items-start gap-2 px-4 pb-3 text-xs text-status-warning">
          <CircleAlert className="w-3.5 h-3.5 shrink-0 mt-0.5" />
          <span>
            Notificações bloqueadas neste navegador. Habilite nas configurações do site (ícone de cadeado na barra de endereço).
          </span>
        </div>
      )}

      {enabled && permission === "granted" && (
        <div className="px-4 pb-3">
          <Button variant="outline" size="sm" className="w-full text-xs" onClick={onTest}>
            Testar notificação
          </Button>
        </div>
      )}
    </div>
  );
}
