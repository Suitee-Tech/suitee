import { ActivitySquare, AlertTriangle, CheckCheck, ChevronLeft, Clock, Hash, Loader2, MessageSquare, Pin, Search, Shield, X } from "lucide-react";

import type { Message } from "../../services/message.service";
import { Button } from "../../ui/button";
import { highlightText } from "./presentation";

export interface MessageMetrics {
  total_messages?: number;
  total_channels?: number;
  avg_response_time_minutes?: number;
  flagged_messages?: number;
}

interface MetricsPanelProps {
  metrics: MessageMetrics | null;
  onClose: () => void;
}

export function MetricsPanel({ metrics, onClose }: MetricsPanelProps) {
  return (
        <div className="flex-1 flex flex-col min-w-0 bg-muted h-full overflow-y-auto">
          <div className="flex-none h-[68px] flex items-center justify-between px-4 md:px-6 border-b border-border bg-card ">
            <div className="flex items-center gap-2">
              <Button variant="ghost" size="icon" onClick={() => onClose()} className="md:hidden -ml-2 size-9 text-muted-foreground">
                <ChevronLeft className="w-5 h-5" />
              </Button>
              <h2 className="text-lg font-bold text-foreground dark:text-white flex items-center gap-2">
                <ActivitySquare className="w-5 h-5 text-primary-500" />
                Painel Administrativo do Chat
              </h2>
            </div>
            <Button variant="ghost" size="icon" onClick={() => onClose()} className="size-9 text-muted-foreground">
              <X className="w-5 h-5" />
            </Button>
          </div>
          <div className="p-6 max-w-5xl mx-auto w-full space-y-6">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
              <div className="bg-card p-5 rounded-2xl border border-border shadow-sm">
                <div className="flex items-center justify-between mb-3">
                  <h3 className="text-sm font-semibold text-muted-foreground">Total de Mensagens</h3>
                  <div className="p-2 bg-primary-50 dark:bg-primary-900/30 rounded-lg">
                    <MessageSquare className="w-4 h-4 text-primary-600 dark:text-primary-400" />
                  </div>
                </div>
                <div className="text-2xl font-bold text-foreground dark:text-white">{metrics?.total_messages?.toLocaleString() || "0"}</div>
              </div>

              <div className="bg-card p-5 rounded-2xl border border-border shadow-sm">
                <div className="flex items-center justify-between mb-3">
                  <h3 className="text-sm font-semibold text-muted-foreground">Canais Ativos</h3>
                  <div className="p-2 bg-status-ok-soft rounded-lg">
                    <Hash className="w-4 h-4 text-status-ok" />
                  </div>
                </div>
                <div className="text-2xl font-bold text-foreground dark:text-white">{metrics?.total_channels?.toLocaleString() || "0"}</div>
              </div>

              <div className="bg-card p-5 rounded-2xl border border-border shadow-sm">
                <div className="flex items-center justify-between mb-3">
                  <h3 className="text-sm font-semibold text-muted-foreground">Tempo Médio Resp.</h3>
                  <div className="p-2 bg-status-warning-soft rounded-lg">
                    <Clock className="w-4 h-4 text-status-warning" />
                  </div>
                </div>
                <div className="text-2xl font-bold text-foreground dark:text-white">
                  {metrics?.avg_response_time_minutes || "0"} <span className="text-sm font-medium text-muted-foreground">min</span>
                </div>
              </div>

              <div className="bg-card p-5 rounded-2xl border border-border shadow-sm">
                <div className="flex items-center justify-between mb-3">
                  <h3 className="text-sm font-semibold text-muted-foreground">Msgs Sinalizadas</h3>
                  <div className="p-2 bg-status-critical-soft rounded-lg">
                    <AlertTriangle className="w-4 h-4 text-status-critical" />
                  </div>
                </div>
                <div className="text-2xl font-bold text-status-critical">{metrics?.flagged_messages || "0"}</div>
              </div>
            </div>

            <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
              <div className="px-5 py-4 border-b border-border flex justify-between items-center bg-muted/50 /50">
                <h3 className="font-bold text-foreground dark:text-white flex items-center gap-2">
                  <Shield className="w-4 h-4 text-status-critical" />
                  Fila de Moderação
                </h3>
                <span className="px-2 py-1 bg-status-critical-soft text-status-critical text-caption font-bold rounded-md">{metrics?.flagged_messages || 0} pendentes</span>
              </div>
              <div className="p-5">
                {(metrics?.flagged_messages ?? 0) > 0 ? (
                  <div className="space-y-3">
                    <div className="flex items-start gap-3 p-3 rounded-xl border border-status-critical/30 bg-status-critical-soft/50">
                      <div className="w-8 h-8 rounded-full bg-muted flex-shrink-0 flex items-center justify-center text-xs font-bold">BR</div>
                      <div className="flex-1">
                        <div className="flex justify-between items-center mb-1">
                          <span className="text-sm font-bold text-foreground dark:text-white">Bruno Silva</span>
                          <span className="text-xs text-muted-foreground">Há 5 min no canal "Negócio #123"</span>
                        </div>
                        <p className="text-sm text-foreground ">
                          Isso é uma <span className="bg-status-critical-soft/60 px-1 rounded">fraude</span>, não vou assinar!
                        </p>
                        <div className="flex gap-2 mt-3">
                          <Button variant="destructive" size="sm">
                            Apagar e Bloquear
                          </Button>
                          <Button variant="outline" size="sm">
                            Ignorar
                          </Button>
                        </div>
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="text-center py-8">
                    <CheckCheck className="w-8 h-8 text-status-ok mx-auto mb-2" />
                    <p className="text-sm font-medium text-muted-foreground">Nenhuma mensagem aguardando moderação</p>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
  );
}

interface SearchResultsPanelProps {
  messageSearch: string;
  setMessageSearch: (value: string) => void;
  searchResults: Message[];
  setSearchResults: (messages: Message[]) => void;
  setHasSearched: (value: boolean) => void;
  isSearchingMessages: boolean;
  handleSelectChannel: (id: string) => void;
  resolveParticipantName: (id: string, name?: string) => string;
}

export function SearchResultsPanel({
  messageSearch,
  setMessageSearch,
  searchResults,
  setSearchResults,
  setHasSearched,
  isSearchingMessages,
  handleSelectChannel,
  resolveParticipantName,
}: SearchResultsPanelProps) {
  return (
        <div className="flex-1 flex flex-col min-w-0 bg-muted h-full overflow-y-auto">
          <div className="flex-none h-[68px] flex items-center justify-between px-4 md:px-6 border-b border-border bg-card ">
            <div className="flex items-center gap-2">
              <Button
                variant="ghost"
                size="icon"
                onClick={() => {
                  setMessageSearch("");
                  setSearchResults([]);
                  setHasSearched(false);
                }}
                className="md:hidden -ml-2 size-9 text-muted-foreground"
              >
                <ChevronLeft className="w-5 h-5" />
              </Button>
              <h2 className="text-lg font-bold text-foreground dark:text-white flex items-center gap-2">
                <Search className="w-5 h-5 text-primary-500" />
                Resultados da Busca
              </h2>
            </div>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => {
                setMessageSearch("");
                setSearchResults([]);
              }}
              className="size-9 text-muted-foreground"
            >
              <X className="w-5 h-5" />
            </Button>
          </div>
          <div className="p-6 max-w-3xl mx-auto w-full space-y-4">
            {isSearchingMessages ? (
              <div className="flex justify-center py-10">
                <Loader2 className="w-8 h-8 text-primary-500 animate-spin" />
              </div>
            ) : searchResults.length === 0 ? (
              <div className="text-center py-10 text-muted-foreground">Nenhum resultado encontrado.</div>
            ) : (
              searchResults.map((msg) => (
                <div
                  key={msg.id}
                  className="bg-card p-5 rounded-2xl shadow-sm border border-border cursor-pointer hover:border-primary-300 hover:shadow-md transition-all"
                  onClick={() => {
                    handleSelectChannel(msg.channel_id);
                    setMessageSearch("");
                    setSearchResults([]);
                  }}
                >
                  <div className="flex justify-between items-center mb-4">
                    <span className="font-bold text-sm text-foreground dark:text-white capitalize flex items-center gap-1">
                      {highlightText(resolveParticipantName(msg.sender_id, msg.sender_name), messageSearch)}
                      {msg.is_pinned && <Pin className="w-3 h-3 text-status-warning" />}
                    </span>
                    <span className="text-xs text-muted-foreground font-medium">{new Date(msg.sent_at).toLocaleString()}</span>
                  </div>
                  <div className="text-sm text-foreground bg-status-warning-soft/50 p-3 rounded-lg border border-status-warning/30 whitespace-pre-wrap">
                    {highlightText(msg.content || "", messageSearch)}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
  );
}
