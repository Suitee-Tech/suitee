import { useEffect, useState } from "react";
import { BellRing, CircleAlert } from "lucide-react";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "../ui/dialog";
import { Button } from "../ui/button";
import { useSystemNotifications } from "../notifications/useSystemNotifications";
import {
  getPermission,
  hasAskedSystemNotificationPermission,
  markSystemNotificationPermissionAsked,
  playAlertChime,
  showSystemNotification,
} from "../notifications/systemNotifications";

export interface SystemNotificationPermissionPromptProps {
  /**
   * Atraso antes de abrir o prompt. Existe para não empilhar com os modais de
   * login/termos que aparecem na chegada do usuário (melhor prática: pedir
   * permissão num momento de engajamento, nunca no load).
   */
  delayMs?: number;
}

/**
 * Soft ask da permissão de notificações — o passo recomendado ANTES do prompt
 * do navegador: um diálogo contextual que explica o benefício; o prompt nativo
 * (`Notification.requestPermission`) só dispara no clique de "Ativar", dentro
 * do gesto do usuário. Mostrado uma única vez por dispositivo.
 */
export function SystemNotificationPermissionPrompt({ delayMs = 15000 }: SystemNotificationPermissionPromptProps) {
  const sys = useSystemNotifications();
  const [open, setOpen] = useState(false);
  const [justDenied, setJustDenied] = useState(false);

  useEffect(() => {
    if (!sys.supported) return;
    if (getPermission() !== "default") return; // granted/denied: nada a pedir
    if (hasAskedSystemNotificationPermission()) return; // já perguntamos neste dispositivo
    const timer = window.setTimeout(() => setOpen(true), delayMs);
    return () => window.clearTimeout(timer);
  }, [sys.supported, delayMs]);

  const handleActivate = async () => {
    const result = await sys.enable(); // desbloqueia áudio + requestPermission no gesto
    markSystemNotificationPermissionAsked();
    if (result === "granted") {
      setOpen(false);
      // Confirmação tangível: notificação de teste + som conforme a preferência.
      void showSystemNotification({
        title: "Suitee",
        body: "Notificações ativadas com sucesso.",
        tag: "suitee:test",
        sound: sys.sound,
      });
      if (sys.sound) playAlertChime();
    } else {
      setJustDenied(true);
    }
  };

  const handleDismiss = () => {
    markSystemNotificationPermissionAsked();
    setOpen(false);
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) handleDismiss();
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <BellRing className="w-5 h-5 text-primary" aria-hidden="true" />
            Receba alertas importantes
          </DialogTitle>
          <DialogDescription>
            {justDenied ? (
              <span className="flex items-start gap-2">
                <CircleAlert className="w-4 h-4 shrink-0 mt-0.5 text-status-warning" aria-hidden="true" />
                O navegador bloqueou a permissão. Você pode liberar depois nas configurações do site (ícone de cadeado na barra de endereço) ou pelo sino de notificações.
              </span>
            ) : (
              "Ative as notificações do navegador para ser avisado de novos leads, chamados e mensagens — com som — mesmo com o Suitee em outra aba."
            )}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter className="gap-2">
          {justDenied ? (
            <Button onClick={() => setOpen(false)}>Entendi</Button>
          ) : (
            <>
              <Button variant="ghost" onClick={handleDismiss}>
                Agora não
              </Button>
              <Button onClick={() => void handleActivate()}>Ativar notificações</Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
