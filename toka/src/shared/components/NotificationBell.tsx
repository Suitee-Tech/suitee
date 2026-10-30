import { useState, useEffect, useCallback } from "react";
import {
  Bell,
  CheckCheck,
  CircleAlert,
  CircleUser,
  Info,
  LifeBuoy,
  MessageSquareText,
  CheckCircle2,
  Wallet,
  X,
} from "lucide-react";
import { clsx } from "clsx";
import { useNavigate } from "react-router";
import { formatDistanceToNow } from "date-fns";
import { ptBR } from "date-fns/locale";
import { toast } from "sonner";

import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { ScrollArea } from "../ui/scroll-area";
import { Button } from "../ui/button";
import { Badge } from "../ui/badge";
import { notificationService, type InAppNotification } from "../services/notification.service";
import { useWsSubscription } from "../ws/useWsSubscription";
import { SystemNotificationControls } from "./SystemNotificationControls";
import { EmailPreferencesPanel } from "./EmailPreferencesPanel";
import { emailPrefsService } from "../services/email-prefs.service";
import { useSystemNotifications } from "../notifications/useSystemNotifications";
import {
  playAlertChime,
  shouldNotifyFromDocumentState,
  showSystemNotification,
  subscribeSystemNotificationClick,
} from "../notifications/systemNotifications";

type NotificationBellProps = {
  userId?: string | null;
};

/** Rota interna da notificação — compartilhada entre o clique no sino e o clique na notificação de sistema. */
function linkForInAppNotification(notif: InAppNotification): string | undefined {
  if (notif.type === "chat_message" && (notif.data as any)?.channel_id) {
    return `/mensagens?channel=${(notif.data as any).channel_id}`;
  }
  if (notif.type && notif.type.startsWith("ticket_")) {
    const ticketID = (notif.data as any)?.ticket_id;
    if (typeof ticketID === "string" && ticketID !== "") {
      return `/chamados/${ticketID}`;
    }
    return notif.link || "/chamados";
  }
  return notif.link;
}

export function NotificationBell({ userId }: NotificationBellProps) {
  const [notifications, setNotifications] = useState<InAppNotification[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [isOpen, setIsOpen] = useState(false);
  const navigate = useNavigate();
  const sys = useSystemNotifications();

  // Clique numa notificação de sistema navega pelo router (SPA, sem reload).
  useEffect(() => subscribeSystemNotificationClick((to) => navigate(to)), [navigate]);

  const fetchInitialData = useCallback(async () => {
    try {
      const [countRes, listRes] = await Promise.all([
        notificationService.getUnreadCount(),
        notificationService.getList(10, 0),
      ]);
      setUnreadCount(countRes.data.count);
      setNotifications(listRes.data || []);
    } catch (err) {
      console.error("Failed to fetch notifications", err);
    }
  }, []);

  useEffect(() => {
    if (userId) {
      fetchInitialData();
    }
  }, [userId, fetchInitialData]);

  const audibleTypes = new Set([
    "chat_message",
    "lead_assigned",
    "ticket_assigned",
    "ticket_comment_added",
    "ticket_status_changed",
    "transfer_processed",
  ]);

  useWsSubscription("inapp_notification", (event) => {
    if (!userId) return;
    const newNotif = (event as any).payload as InAppNotification;
    setNotifications((prev) => [newNotif, ...prev]);
    setUnreadCount((prev) => prev + 1);

    const audible = newNotif.priority === "urgent" || newNotif.priority === "important" || audibleTypes.has(newNotif.type);
    const hidden = shouldNotifyFromDocumentState();

    // Aba visível: alerta sonoro in-app (Web Notifications não aceita som
    // customizado — só o som padrão do sistema, que toca quando hidden).
    if (audible && sys.sound && !hidden) {
      playAlertChime();
    }

    // Melhor prática: notificação de sistema apenas com a aba em segundo
    // plano; com a página visível o toast in-app é o canal certo.
    if (hidden && sys.enabled) {
      void showSystemNotification({
        title: newNotif.title,
        body: newNotif.content,
        tag: `inapp:${newNotif.type}:${newNotif.id}`,
        link: linkForInAppNotification(newNotif),
        sound: sys.sound,
      });
    }

    switch (newNotif.type) {
      case "lead_assigned":
        toast.success(newNotif.title, {
          description: newNotif.content,
          action: {
            label: "Ver Lead",
            onClick: () => navigate(newNotif.link || "/leads"),
          },
        });
        break;
      case "ticket_assigned":
        toast.info(newNotif.title, {
          description: newNotif.content,
          action: {
            label: "Ver Chamado",
            onClick: () => navigate(newNotif.link || "/chamados"),
          },
        });
        break;
      case "ticket_comment_added":
        toast.message(newNotif.title, {
          description: newNotif.content,
          action: {
            label: "Abrir Chamado",
            onClick: () => navigate(newNotif.link || "/chamados"),
          },
        });
        break;
      case "ticket_status_changed":
        toast.success(newNotif.title, {
          description: newNotif.content,
          action: {
            label: "Ver detalhes",
            onClick: () => navigate(newNotif.link || "/chamados"),
          },
        });
        break;
      case "transfer_processed":
        toast.success(newNotif.title, {
          description: newNotif.content,
          action: {
            label: "Ver Repasse",
            onClick: () => navigate(newNotif.link || "/portal/finance"),
          },
        });
        break;
    }
  });

  const handleMarkAsRead = async (id: string) => {
    try {
      await notificationService.markAsRead(id);
      setNotifications((prev) => prev.map((n) => (n.id === id ? { ...n, is_read: true } : n)));
      setUnreadCount((prev) => Math.max(0, prev - 1));
    } catch (err) {
      console.error(err);
    }
  };

  const handleMarkAllAsRead = async () => {
    try {
      await notificationService.markAllAsRead();
      setNotifications((prev) => prev.map((n) => ({ ...n, is_read: true })));
      setUnreadCount(0);
    } catch (err) {
      console.error(err);
    }
  };

  const handleNotificationClick = (notif: InAppNotification) => {
    if (!notif.is_read) {
      handleMarkAsRead(notif.id);
    }
    setIsOpen(false);
    const link = linkForInAppNotification(notif);
    if (link) {
      navigate(link);
    }
  };

  const getPriorityColor = (priority: string) => {
    switch (priority) {
      case "urgent":
        return "text-destructive";
      case "important":
        return "text-warning";
      default:
        return "text-muted-foreground";
    }
  };

  const getIcon = (type: string, priority: string) => {
    switch (type) {
      case "chat_message":
        return <CircleUser className="w-5 h-5 text-primary" />;
      case "lead_assigned":
        return <CircleAlert className={`w-5 h-5 ${getPriorityColor(priority)}`} />;
      case "ticket_assigned":
        return <LifeBuoy className={`w-5 h-5 ${getPriorityColor(priority)}`} />;
      case "ticket_comment_added":
        return <MessageSquareText className="w-5 h-5 text-primary" />;
      case "ticket_status_changed":
        return <CheckCircle2 className="w-5 h-5 text-status-ok" />;
      case "transfer_processed":
        return <Wallet className="w-5 h-5 text-status-ok" />;
      default:
        return <Info className="w-5 h-5 text-muted-foreground" />;
    }
  };

  const [filter, setFilter] = useState<"all" | "unread">("unread");
  const [view, setView] = useState<"inbox" | "settings">("inbox");

  const filteredNotifications = notifications.filter((n) => {
    if (filter === "unread" && n.is_read) return false;
    return true;
  });

  return (
    <Popover open={isOpen} onOpenChange={setIsOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={unreadCount > 0 ? `Notificações: ${unreadCount} não lidas` : "Notificações"}
          className={clsx("relative p-2 rounded-full transition-colors", "text-muted-foreground hover:bg-muted")}
        >
          <Bell aria-hidden="true" className="w-5 h-5" />
          {unreadCount > 0 && (
            <span className="absolute top-1 right-1 flex items-center justify-center w-4 h-4 bg-destructive text-micro text-destructive-foreground font-bold rounded-full border-2 border-background">
              {unreadCount > 99 ? "99+" : unreadCount}
            </span>
          )}
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-[calc(100vw-2rem)] max-w-[calc(100vw-2rem)] p-0 sm:w-[420px]">
        <header className="flex items-start justify-between gap-2 border-b border-border p-4">
          <div className="min-w-0">
            <h4 className="text-sm font-semibold text-foreground">Notificações</h4>
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

        <div className="flex border-b border-border px-3" role="tablist" aria-label="Notificações e preferências">
          <button
            type="button"
            role="tab"
            aria-selected={view === "inbox"}
            onClick={() => setView("inbox")}
            className={clsx(
              "flex flex-1 items-center justify-center gap-1.5 border-b-2 px-3 py-2 text-sm font-semibold transition-colors",
              view === "inbox"
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
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
            aria-selected={view === "settings"}
            onClick={() => setView("settings")}
            className={clsx(
              "flex flex-1 items-center justify-center border-b-2 px-3 py-2 text-sm font-semibold transition-colors",
              view === "settings"
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            Preferências
          </button>
        </div>

        {view === "inbox" ? (
          <>
            <div className="flex flex-wrap items-center gap-1.5 border-b border-border px-3 py-2">
              <Button
                variant="ghost"
                size="sm"
                className={clsx("h-7 px-2 text-xs", filter === "all" && "bg-muted")}
                aria-pressed={filter === "all"}
                onClick={() => setFilter("all")}
              >
                Todas
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className={clsx("h-7 px-2 text-xs", filter === "unread" && "bg-muted")}
                aria-pressed={filter === "unread"}
                onClick={() => setFilter("unread")}
              >
                Não lidas
              </Button>
              {unreadCount > 0 && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="ml-auto h-7 px-2 text-xs"
                  onClick={handleMarkAllAsRead}
                >
                  <CheckCheck className="mr-1 h-3 w-3" />
                  Ler tudo
                </Button>
              )}
            </div>

            <ScrollArea className="max-h-[min(55vh,440px)]">
              <div className="p-2">
                {filteredNotifications.length === 0 ? (
                  <div className="flex flex-col items-center gap-1 px-6 py-10 text-center">
                    <Bell className="h-8 w-8 opacity-30" />
                    <p className="text-sm font-medium text-foreground">
                      {filter === "unread" ? "Tudo em dia" : "Nenhuma notificação"}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {filter === "unread"
                        ? "Você não tem notificações não lidas."
                        : "As novidades aparecerão aqui."}
                    </p>
                  </div>
                ) : (
                  filteredNotifications.map((notif) => (
                    <button
                      key={notif.id}
                      type="button"
                      onClick={() => handleNotificationClick(notif)}
                      className={clsx(
                        "mb-1 flex w-full gap-2.5 rounded-xl p-3 text-left transition-colors",
                        notif.is_read ? "hover:bg-muted/50" : "bg-primary/5 hover:bg-primary/10",
                      )}
                    >
                      <span
                        className={clsx(
                          "mt-1.5 h-2 w-2 shrink-0 rounded-full",
                          notif.is_read ? "bg-transparent" : "bg-primary",
                        )}
                        aria-hidden="true"
                      />
                      <div className="mt-0.5 shrink-0">{getIcon(notif.type, notif.priority)}</div>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-start justify-between gap-2">
                          <p className="truncate text-sm font-medium text-foreground">{notif.title}</p>
                          {!notif.is_read && (
                            <Badge className="shrink-0 px-1.5 py-0.5 text-micro">Novo</Badge>
                          )}
                        </div>
                        <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
                          {notif.content}
                        </p>
                        <p className="mt-1 text-micro text-muted-foreground">
                          {formatDistanceToNow(new Date(notif.created_at), {
                            addSuffix: true,
                            locale: ptBR,
                          })}
                        </p>
                      </div>
                    </button>
                  ))
                )}
              </div>
            </ScrollArea>
          </>
        ) : (
          <ScrollArea className="max-h-[min(65vh,560px)]">
            <div className="space-y-3 p-3">
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
              <EmailPreferencesPanel service={emailPrefsService} />
            </div>
          </ScrollArea>
        )}
      </PopoverContent>
    </Popover>
  );
}
