import { useCallback, useEffect, useState } from "react";
import { CheckCheck, CircleAlert, CircleUser, Info, BellRing } from "lucide-react";
import { clsx } from "clsx";
import { useNavigate } from "react-router";
import { Button } from "../ui/button";
import { Badge } from "../ui/badge";
import { notificationService, type InAppNotification } from "../services/notification.service";
import { formatDistanceToNow } from "date-fns";
import { ptBR } from "date-fns/locale";

export function Notifications() {
  const [notifications, setNotifications] = useState<InAppNotification[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [filter, setFilter] = useState<"all" | "unread">("all");
  const navigate = useNavigate();

  const fetchNotifications = useCallback(async () => {
    setIsLoading(true);
    try {
      const listRes = await notificationService.getList(100, 0);
      setNotifications(listRes.data || []);
    } catch {
      // silent
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchNotifications();
  }, [fetchNotifications]);

  const handleMarkAsRead = async (id: string) => {
    try {
      await notificationService.markAsRead(id);
      setNotifications((prev) => prev.map((n) => (n.id === id ? { ...n, is_read: true } : n)));
    } catch {
      // silent
    }
  };

  const handleMarkAllAsRead = async () => {
    try {
      await notificationService.markAllAsRead();
      setNotifications((prev) => prev.map((n) => ({ ...n, is_read: true })));
    } catch {
      // silent
    }
  };

  const handleNotificationClick = (notif: InAppNotification) => {
    if (!notif.is_read) handleMarkAsRead(notif.id);

    if (notif.type === "chat_message" && notif.data?.channel_id) {
      navigate(`/mensagens?channel=${notif.data.channel_id}`);
    } else if (notif.link) {
      navigate(notif.link);
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
    if (type === "chat_message") return <CircleUser className="w-5 h-5 text-primary" />;
    if (type === "lead_assigned")
      return <CircleAlert className={`w-5 h-5 ${getPriorityColor(priority)}`} />;
    return <Info className="w-5 h-5 text-muted-foreground" />;
  };

  const filteredNotifications = notifications.filter((n) => {
    if (filter === "unread" && n.is_read) return false;
    return true;
  });

  const unreadCount = notifications.filter((n) => !n.is_read).length;

  return (
    <div className="flex-1 flex flex-col min-w-0 bg-muted/30 h-full overflow-y-auto">
      <div className="flex-none flex flex-col sm:flex-row sm:items-center justify-between p-4 sm:px-6 sm:h-[68px] gap-4 border-b border-border bg-card">
        <div className="flex items-center gap-3 min-w-0">
          <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center shrink-0">
            <BellRing className="w-5 h-5 text-primary" />
          </div>
          <div className="min-w-0">
            <h2 className="text-lg font-bold text-foreground truncate">Central de Notificações</h2>
            <p className="text-xs text-muted-foreground truncate">Gerencie seus alertas e mensagens</p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <div className="bg-muted p-1 rounded-lg flex items-center shrink-0">
            <button
              onClick={() => setFilter("all")}
              className={clsx(
                "px-3 py-1.5 text-xs font-medium rounded-md transition-all",
                filter === "all"
                  ? "bg-card shadow-sm text-foreground"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              Todas
            </button>
            <button
              onClick={() => setFilter("unread")}
              className={clsx(
                "px-3 py-1.5 text-xs font-medium rounded-md transition-all",
                filter === "unread"
                  ? "bg-card shadow-sm text-foreground"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              Não lidas
            </button>
          </div>

          {unreadCount > 0 && (
            <Button
              variant="outline"
              size="sm"
              onClick={handleMarkAllAsRead}
              className="text-xs shrink-0"
            >
              <CheckCheck className="w-4 h-4 mr-2 shrink-0" />
              Marcar {unreadCount} como lidas
            </Button>
          )}
        </div>
      </div>

      <div className="flex-1 p-6 max-w-4xl mx-auto w-full">
        {isLoading ? (
          <div className="flex items-center justify-center py-20">
            <div className="w-8 h-8 border-4 border-primary/30 border-t-primary rounded-full animate-spin" />
          </div>
        ) : filteredNotifications.length === 0 ? (
          <div className="text-center py-20 bg-card rounded-2xl border border-border shadow-sm">
            <BellRing className="w-12 h-12 text-muted-foreground/30 mx-auto mb-4" />
            <h3 className="text-lg font-semibold text-foreground mb-2">Nenhuma notificação</h3>
            <p className="text-sm text-muted-foreground">
              Você está em dia com todas as suas atualizações.
            </p>
          </div>
        ) : (
          <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden flex flex-col divide-y divide-border">
            {filteredNotifications.map((notif) => (
              <div
                key={notif.id}
                onClick={() => handleNotificationClick(notif)}
                className={clsx(
                  "p-5 flex gap-4 cursor-pointer transition-colors hover:bg-muted/50",
                  !notif.is_read ? "bg-primary/[0.02]" : "opacity-75 hover:opacity-100",
                )}
              >
                <div
                  className={clsx(
                    "shrink-0 w-10 h-10 rounded-full flex items-center justify-center",
                    !notif.is_read
                      ? "bg-primary/10 text-primary"
                      : "bg-muted text-muted-foreground",
                  )}
                >
                  {getIcon(notif.type, notif.priority)}
                </div>

                <div className="flex-1 min-w-0">
                  <div className="flex items-start justify-between gap-4">
                    <p
                      className={clsx(
                        "text-sm font-semibold",
                        !notif.is_read ? "text-foreground" : "text-muted-foreground",
                      )}
                    >
                      {notif.title}
                    </p>
                    <span className="text-xs text-muted-foreground whitespace-nowrap">
                      {formatDistanceToNow(new Date(notif.created_at), {
                        addSuffix: true,
                        locale: ptBR,
                      })}
                    </span>
                  </div>

                  <p
                    className={clsx(
                      "text-sm mt-1",
                      !notif.is_read ? "text-muted-foreground" : "text-muted-foreground/70",
                    )}
                  >
                    {notif.content}
                  </p>

                  <div className="flex items-center gap-2 mt-3">
                    {notif.priority === "urgent" && (
                      <Badge variant="destructive" className="text-caption px-2 py-0 h-5">
                        Urgente
                      </Badge>
                    )}
                    {notif.type === "chat_message" && (
                      <Badge
                        variant="outline"
                        className="text-caption px-2 py-0 h-5 bg-status-info-soft text-status-info border-status-info/30"
                      >
                        Chat
                      </Badge>
                    )}
                    {notif.type === "lead_assigned" && (
                      <Badge
                        variant="outline"
                        className="text-caption px-2 py-0 h-5 bg-status-warning-soft text-status-warning border-status-warning/30"
                      >
                        CRM
                      </Badge>
                    )}
                  </div>
                </div>

                {!notif.is_read && (
                  <div className="shrink-0 flex items-center">
                    <div className="w-2.5 h-2.5 rounded-full bg-primary" />
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

