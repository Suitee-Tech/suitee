import { useEffect, useState } from "react";
import { Bell, Check, ExternalLink, Loader2, RefreshCw, X } from "lucide-react";
import { Link, useNavigate } from "react-router";
import { formatDistanceToNow } from "date-fns";
import { ptBR } from "date-fns/locale";
import { Popover, PopoverContent, PopoverTrigger } from "../../../shared/ui/popover";
import { Button } from "../../../shared/ui/button";
import { getPortalNotificationLink, portalNotificationService, PortalNotification } from "../services/portal-notification.service";
import { usePortalWebSocket } from "../hooks/usePortalWebSocket";
import { SystemNotificationControls } from "../../../shared/components/SystemNotificationControls";
import { EmailPreferencesPanel } from "../../../shared/components/EmailPreferencesPanel";
import { portalEmailPrefsService } from "../services/portal-email-prefs.service";
import { useSystemNotifications } from "../../../shared/notifications/useSystemNotifications";
import {
  playAlertChime,
  shouldNotifyFromDocumentState,
  showSystemNotification,
  subscribeSystemNotificationClick,
} from "../../../shared/notifications/systemNotifications";

export function NotificationCenter() {
  const [notifications, setNotifications] = useState<PortalNotification[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [isOpen, setIsOpen] = useState(false);
  const [view, setView] = useState<"inbox" | "preferences">("inbox");
  const [isLoading, setIsLoading] = useState(true);
  const [hasError, setHasError] = useState(false);
  const { onNotification } = usePortalWebSocket();
  const navigate = useNavigate();
  const sys = useSystemNotifications();

  // Clique numa notificação de sistema navega pelo router (SPA, sem reload).
  useEffect(() => subscribeSystemNotificationClick((to) => navigate(to)), [navigate]);

  const fetchNotifications = async () => {
    setIsLoading(true);
    setHasError(false);
    try {
      const [listRes, count] = await Promise.all([portalNotificationService.getList(10, 0), portalNotificationService.getUnreadCount()]);
      setNotifications(listRes.items || []);
      setUnreadCount(count || 0);
    } catch {
      setHasError(true);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchNotifications();
  }, []);

  useEffect(() => {
    const unsubscribe = onNotification((payload: PortalNotification) => {
      setNotifications((prev) => [payload, ...prev].slice(0, 10));
      setUnreadCount((prev) => prev + 1);

      const hidden = shouldNotifyFromDocumentState();
      if (sys.sound && !hidden) playAlertChime();
      if (hidden && sys.enabled) {
        const to = getPortalNotificationLink(payload);
        void showSystemNotification({
          title: payload.title,
          body: payload.message || payload.content || "",
          tag: `portal:${payload.type}:${payload.id}`,
          link: to ? to.pathname + (to.search || "") : payload.link,
          sound: sys.sound,
        });
      }
    });
    return () => unsubscribe();
  }, [onNotification, sys.enabled, sys.sound]);

  useEffect(() => {
    if (!isOpen) setView("inbox");
  }, [isOpen]);

  const handleMarkAsRead = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await portalNotificationService.markAsRead(id);
      setNotifications((prev) => prev.map((n) => (n.id === id ? { ...n, is_read: true } : n)));
      setUnreadCount((prev) => Math.max(0, prev - 1));
    } catch {}
  };

  const handleOpenNotification = async (n: PortalNotification) => {
    try {
      if (!n.is_read) {
        await portalNotificationService.markAsRead(n.id);
        setNotifications((prev) => prev.map((x) => (x.id === n.id ? { ...x, is_read: true } : x)));
        setUnreadCount((prev) => Math.max(0, prev - 1));
      }
    } catch {}
    setIsOpen(false);
  };

  const handleMarkAllAsRead = async () => {
    try {
      await portalNotificationService.markAllAsRead();
      setNotifications((prev) => prev.map((n) => ({ ...n, is_read: true })));
      setUnreadCount(0);
    } catch {}
  };

  return (
    <Popover open={isOpen} onOpenChange={setIsOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={unreadCount > 0 ? `Notificações: ${unreadCount} não lidas` : "Notificações"}
          className="relative w-10 h-10 rounded-full bg-card border border-border flex items-center justify-center text-muted-foreground hover:bg-muted/50 transition-colors"
        >
          <Bell className="w-5 h-5" />
          {unreadCount > 0 && <span className="absolute top-2 right-2 w-2 h-2 bg-status-critical rounded-full border border-white" />}
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-[calc(100vw-2rem)] max-w-[calc(100vw-2rem)] overflow-hidden rounded-2xl p-0 sm:w-[420px]">
        <header className="flex items-start justify-between gap-2 border-b border-border p-4">
          <div className="min-w-0">
            <h3 className="text-title font-bold text-foreground">Notificações</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {unreadCount > 0
                ? `${unreadCount} não lida${unreadCount === 1 ? "" : "s"}`
                : "Tudo em dia"}
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 shrink-0"
            onClick={() => setIsOpen(false)}
            aria-label="Fechar notificações"
          >
            <X className="h-4 w-4" />
          </Button>
        </header>

        <div className="grid grid-cols-2 border-b border-border" role="tablist" aria-label="Notificações e preferências">
          <button
            type="button"
            role="tab"
            aria-selected={view === "inbox"}
            onClick={() => setView("inbox")}
            className={`flex items-center justify-center gap-1.5 border-b-2 px-3 py-2 text-sm font-semibold transition-colors ${
              view === "inbox"
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            Notificações
            {unreadCount > 0 && (
              <span className="rounded-full bg-primary/10 px-1.5 py-0.5 text-micro font-bold text-primary">
                {unreadCount > 99 ? "99+" : unreadCount}
              </span>
            )}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={view === "preferences"}
            onClick={() => setView("preferences")}
            className={`border-b-2 px-3 py-2 text-sm font-semibold transition-colors ${
              view === "preferences"
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            Preferências
          </button>
        </div>

        {view === "inbox" ? (
          <>
            <div className="max-h-[min(55vh,440px)] overflow-y-auto">
              {isLoading ? (
                <div className="flex items-center justify-center gap-2 p-6 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" /> Carregando notificações...
                </div>
              ) : hasError ? (
                <div className="p-6 text-center">
                  <p className="text-sm text-muted-foreground">Não foi possível carregar as notificações.</p>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => void fetchNotifications()}
                    className="mt-2 text-xs font-semibold"
                  >
                    <RefreshCw className="mr-1 h-3.5 w-3.5" /> Tentar novamente
                  </Button>
                </div>
              ) : notifications.length === 0 ? (
                <div className="flex flex-col items-center gap-1 p-8 text-center">
                  <Bell className="h-8 w-8 opacity-30" />
                  <p className="text-sm font-medium text-foreground">Tudo em dia</p>
                  <p className="text-xs text-muted-foreground">As novidades aparecerão aqui.</p>
                </div>
              ) : (
                <div className="divide-y divide-border">
                  {notifications.map((n) => (
                    <div key={n.id} className={!n.is_read ? "bg-primary/5" : ""}>
                      <div className="flex gap-2.5 p-4">
                        <span
                          className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${
                            n.is_read ? "bg-transparent" : "bg-status-ok"
                          }`}
                          aria-hidden="true"
                        />
                        <div className="min-w-0 flex-1">
                          {(() => {
                            const to = getPortalNotificationLink(n);
                            const content = (
                              <>
                                <p className={`text-sm ${!n.is_read ? "font-semibold text-foreground" : "text-foreground/80"}`}>
                                  {n.title}
                                </p>
                                <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{n.message}</p>
                              </>
                            );
                            if (!to) return content;
                            return (
                              <Link
                                to={`${to.pathname}${to.search || ""}`}
                                onClick={() => handleOpenNotification(n)}
                                className="block"
                              >
                                {content}
                              </Link>
                            );
                          })()}
                          <div className="mt-2 flex items-center justify-between">
                            <span className="text-micro font-medium text-muted-foreground/70">
                              {formatDistanceToNow(new Date(n.created_at), {
                                addSuffix: true,
                                locale: ptBR,
                              })}
                            </span>
                            {!n.is_read && (
                              <button
                                type="button"
                                onClick={(e) => handleMarkAsRead(n.id, e)}
                                className="flex items-center gap-1 text-caption font-bold text-status-ok hover:text-status-ok"
                              >
                                <Check className="h-3 w-3" />
                                Marcar como lida
                              </button>
                            )}
                          </div>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="flex gap-2 border-t border-border bg-muted/50 p-3">
              {unreadCount > 0 && (
                <Button variant="outline" size="sm" onClick={handleMarkAllAsRead} className="flex-1 text-xs">
                  Marcar todas como lidas
                </Button>
              )}
              <Button asChild variant="default" size="sm" className="flex-1 text-xs" onClick={() => setIsOpen(false)}>
                <Link to="/portal/notificacoes">
                  Ver todas <ExternalLink className="ml-1 h-3 w-3" />
                </Link>
              </Button>
            </div>
          </>
        ) : (
          <div className="max-h-[min(65vh,560px)] space-y-3 overflow-y-auto p-3">
            <p className="px-1 text-xs text-muted-foreground">
              Escolha como quer ser avisado. As alterações são salvas automaticamente.
            </p>
            <SystemNotificationControls
              supported={sys.supported}
              permission={sys.permission}
              enabled={sys.enabled}
              sound={sys.sound}
              onEnable={sys.enable}
              onSetEnabled={sys.setEnabled}
              onSetSound={sys.setSound}
              onTest={() => {
                void showSystemNotification({
                  title: "Suitee",
                  body: "Notificações do sistema ativadas com sucesso.",
                  tag: "suitee:test",
                  sound: sys.sound,
                });
                if (sys.sound) playAlertChime();
              }}
            />
            <div className="border-t border-border" />
            <EmailPreferencesPanel service={portalEmailPrefsService} />
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}
