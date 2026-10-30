import { useEffect, useMemo, useState } from "react";
import { formatDistanceToNow } from "date-fns";
import { ptBR } from "date-fns/locale";
import { Check, Bell } from "lucide-react";
import { Link } from "react-router";
import {
  getPortalNotificationLink,
  portalNotificationService,
  type PortalNotification,
} from "../services/portal-notification.service";
import { PortalEmpty, PortalError, PortalLoading } from "../components/PortalState";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../../../shared/ui/tabs";
import { Button } from "../../../shared/ui/button";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";
import { SystemNotificationControls } from "../../../shared/components/SystemNotificationControls";
import { EmailPreferencesPanel } from "../../../shared/components/EmailPreferencesPanel";
import { portalEmailPrefsService } from "../services/portal-email-prefs.service";
import { useSystemNotifications } from "../../../shared/notifications/useSystemNotifications";
import {
  playAlertChime,
  showSystemNotification,
} from "../../../shared/notifications/systemNotifications";

export function PortalNotifications() {
  const sys = useSystemNotifications();
  const [notifications, setNotifications] = useState<PortalNotification[]>([]);
  const [loading, setLoading] = useState(true);
  const [hasError, setHasError] = useState(false);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [offset, setOffset] = useState(0);
  const [total, setTotal] = useState(0);
  const limit = 20;
  const unreadCount = useMemo(() => notifications.filter((n) => !n.is_read).length, [notifications]);
  const hasMore =
    total === 0 ? notifications.length > 0 && notifications.length % limit === 0 : notifications.length < total;

  const fetchNotifications = async () => {
    setLoading(true);
    setHasError(false);
    setOffset(0);
    try {
      const listRes = await portalNotificationService.getList(limit, 0);
      setNotifications(listRes.items || []);
      setTotal(listRes.total || 0);
    } catch (err) {
      console.error(err);
      setHasError(true);
    } finally {
      setLoading(false);
    }
  };

  const loadMore = async () => {
    if (isLoadingMore) return;
    const nextOffset = offset + limit;
    if (total > 0 && nextOffset >= total) return;
    setIsLoadingMore(true);
    try {
      const listRes = await portalNotificationService.getList(limit, nextOffset);
      setNotifications((prev) => {
        const ids = new Set(prev.map((n) => n.id));
        const next = (listRes.items || []).filter((n) => !ids.has(n.id));
        return [...prev, ...next];
      });
      setOffset(nextOffset);
      setTotal(listRes.total || total);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingMore(false);
    }
  };

  useEffect(() => {
    fetchNotifications();
  }, []);

  const handleMarkAsRead = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await portalNotificationService.markAsRead(id);
      setNotifications((prev) => prev.map((n) => (n.id === id ? { ...n, is_read: true } : n)));
    } catch (err) {
      console.error(err);
    }
  };

  const handleMarkAllAsRead = async () => {
    try {
      await portalNotificationService.markAllAsRead();
      setNotifications((prev) => prev.map((n) => ({ ...n, is_read: true })));
    } catch (err) {
      console.error(err);
    }
  };

  if (loading) {
    return (
      <PortalPage>
        <PortalPageHeader title="Notificações" />
        <div className="px-5 pt-6">
          <PortalLoading rows={3} header={false} />
        </div>
      </PortalPage>
    );
  }

  if (hasError) {
    return (
      <PortalPage>
        <PortalPageHeader title="Notificações" />
        <div className="px-5 pt-6">
          <PortalError onRetry={fetchNotifications} />
        </div>
      </PortalPage>
    );
  }

  return (
    <PortalPage>
      <PortalPageHeader
        title="Notificações"
        actions={
          unreadCount > 0 ? (
            <Button variant="ghost" size="sm" onClick={handleMarkAllAsRead} className="text-xs font-semibold text-status-ok">
              Marcar todas lidas
            </Button>
          ) : undefined
        }
      />
      <div className="px-5 pt-5">

      <Tabs defaultValue="all" className="w-full">
        <TabsList className="w-full grid w-full grid-cols-3 mb-6">
          <TabsTrigger value="all">Todas</TabsTrigger>
          <TabsTrigger value="unread">
            Não lidas{" "}
            {unreadCount > 0 && (
              <span className="ml-1 bg-status-critical text-status-critical-foreground px-1.5 py-0.5 rounded-full text-micro">
                {unreadCount}
              </span>
            )}
          </TabsTrigger>
          <TabsTrigger value="preferences">Preferências</TabsTrigger>
        </TabsList>

        <TabsContent value="all" className="mt-0">
          <NotificationList notifications={notifications} onMarkAsRead={handleMarkAsRead} />
          {hasMore && (
            <div className="mt-6">
              <Button onClick={loadMore} disabled={isLoadingMore} className="w-full h-11 rounded-xl text-sm font-bold">
                {isLoadingMore ? "Carregando..." : "Carregar mais"}
              </Button>
            </div>
          )}
        </TabsContent>
        <TabsContent value="unread" className="mt-0">
          <NotificationList notifications={notifications.filter((n) => !n.is_read)} onMarkAsRead={handleMarkAsRead} />
          {hasMore && (
            <div className="mt-6">
              <Button onClick={loadMore} disabled={isLoadingMore} className="w-full h-11 rounded-xl text-sm font-bold">
                {isLoadingMore ? "Carregando..." : "Carregar mais"}
              </Button>
            </div>
          )}
        </TabsContent>
        <TabsContent value="preferences" className="mt-0">
          <div className="space-y-3">
            <p className="text-label text-muted-foreground">
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
            <div className="rounded-2xl border border-border bg-card p-4">
              <EmailPreferencesPanel service={portalEmailPrefsService} />
            </div>
          </div>
        </TabsContent>
        </Tabs>
      </div>
    </PortalPage>
  );
}

function NotificationList({
  notifications,
  onMarkAsRead,
}: {
  notifications: PortalNotification[];
  onMarkAsRead: (id: string, e: React.MouseEvent) => void;
}) {
  if (notifications.length === 0) {
    return <PortalEmpty icon={Bell} title="Nenhuma notificação" description="Você não tem notificações nesta categoria." />;
  }

  return (
    <div className="space-y-3">
      {notifications.map((n) => (
        <div
          key={n.id}
          className={`p-4 bg-card border border-border rounded-2xl shadow-sm transition-colors ${!n.is_read ? "border-status-ok/20 bg-status-ok-soft/20" : ""}`}
        >
          <div className="flex gap-3">
            <div className="flex-1 min-w-0">
              {(() => {
                const to = getPortalNotificationLink(n);
                const content = (
                  <>
                    <p className={`text-title ${!n.is_read ? "font-bold text-foreground" : "font-semibold text-foreground/80"}`}>
                      {n.title}
                    </p>
                    <p className="text-label text-muted-foreground mt-1">{n.message}</p>
                  </>
                );
                if (!to) return content;
                return (
                  <Link to={`${to.pathname}${to.search || ""}`} onClick={(e) => onMarkAsRead(n.id, e as any)} className="block">
                    {content}
                  </Link>
                );
              })()}
              <div className="flex items-center justify-between mt-3">
                <span className="text-xs font-medium text-muted-foreground/70">
                  {formatDistanceToNow(new Date(n.created_at), { addSuffix: true, locale: ptBR })}
                </span>
                {!n.is_read && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={(e) => onMarkAsRead(n.id, e)}
                    className="h-auto gap-1 px-2 py-1 text-xs font-bold text-status-ok hover:text-status-ok"
                  >
                    <Check className="w-3.5 h-3.5" />
                    Marcar como lida
                  </Button>
                )}
              </div>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
