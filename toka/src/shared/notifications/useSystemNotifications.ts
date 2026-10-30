import { useCallback, useEffect, useState } from "react";
import {
  getPermission,
  isSupported,
  loadPrefs,
  requestPermission,
  savePrefs,
  unlockAlertAudio,
  type SystemNotificationPermission,
  type SystemNotificationPrefs,
} from "./systemNotifications";

/**
 * Estado das notificações de sistema por dispositivo: permissão do navegador
 * + preferências do usuário (habilitado / som). A permissão é re-lida quando
 * o usuário volta para a aba (pode ter mudado nas configurações do browser).
 */
export function useSystemNotifications() {
  const [permission, setPermission] = useState<SystemNotificationPermission>(getPermission);
  const [prefs, setPrefs] = useState<SystemNotificationPrefs>(loadPrefs);

  useEffect(() => {
    const refresh = () => setPermission(getPermission());
    window.addEventListener("focus", refresh);
    return () => window.removeEventListener("focus", refresh);
  }, []);

  const updatePrefs = useCallback((patch: Partial<SystemNotificationPrefs>) => {
    setPrefs((prev) => {
      const next = { ...prev, ...patch };
      savePrefs(next);
      return next;
    });
  }, []);

  /**
   * Fluxo de ativação: desbloqueia o áudio e pede permissão — ambos dentro do
   * gesto de clique do toggle. Devolve a permissão final para a UI decidir.
   */
  const enable = useCallback(async (): Promise<SystemNotificationPermission> => {
    unlockAlertAudio();
    const result = await requestPermission();
    setPermission(result);
    if (result === "granted") updatePrefs({ enabled: true });
    return result;
  }, [updatePrefs]);

  const setEnabled = useCallback((v: boolean) => updatePrefs({ enabled: v }), [updatePrefs]);

  const setSound = useCallback(
    (v: boolean) => {
      if (v) unlockAlertAudio();
      updatePrefs({ sound: v });
    },
    [updatePrefs],
  );

  return { supported: isSupported(), permission, enabled: prefs.enabled, sound: prefs.sound, enable, setEnabled, setSound };
}
