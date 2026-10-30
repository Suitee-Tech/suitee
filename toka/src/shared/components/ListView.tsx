import { useMemo } from "react";
import { MoreVertical, Mail, Phone, ExternalLink, Calendar } from "lucide-react";
import { useNavigate } from "react-router";
import { Lead, resolvePropertyDisplay } from "../data";
import type { Agent } from "../types/agent";
import { usePropertiesContext } from "../context/PropertiesContext";
import { Avatar, AvatarImage, AvatarFallback } from "../ui/avatar";
import { Button } from "../ui/button";
import { getAvatarThumbUrl } from "../media";
import { formatPhoneBR } from "../utils/masks";
import { LeadAssigneePicker } from "./LeadAssigneePicker";
import { formatBRL, formatLeadDate } from "../lib/leads/format";
import { getLeadTypeDetails } from "../lib/leads/labels";
import { SharedBadge } from "./SharedBadge";

interface ListViewProps {
 leads: Lead[];
 agents: Agent[];
 user?: any;
 canAssign?: boolean;
 onAssignLead: (leadId: string, agentId: string) => void;
}

export function ListView({ leads, agents, user, canAssign, onAssignLead }: ListViewProps) {
 const navigate = useNavigate();
 const { properties } = usePropertiesContext();
 const agentById = useMemo(() => {
  const m: Record<string, Agent> = {};
  for (const a of agents) {
   if (a?.id) m[a.id] = a;
  }
  return m;
 }, [agents]);

 return (
 <div className="bg-card rounded-xl shadow-sm border border-border overflow-hidden h-full flex flex-col">
 <div className="overflow-x-auto">
 <table className="w-full text-left border-collapse" aria-label="Lista de leads">
 <thead>
 <tr className="bg-muted border-b border-border text-xs uppercase font-semibold text-muted-foreground tracking-wider">
 <th className="px-6 py-4">Lead</th>
 <th className="px-6 py-4">Responsável</th>
 <th className="px-6 py-4">Contato</th>
 <th className="px-6 py-4">Interesse (Imóvel)</th>
 <th className="px-6 py-4">Valor do negócio</th>
 <th className="px-6 py-4">Status</th>
 <th className="px-6 py-4">Data</th>
 <th className="px-6 py-4 text-right">Ações</th>
 </tr>
 </thead>
 <tbody className="divide-y divide-border">
 {leads.map((lead) => {
 const formattedBudget = formatBRL(lead.budget);

 const formattedDate = formatLeadDate(lead.date, "short");

 const agent = lead.agentId ? agentById[lead.agentId] : undefined;
 const assignedAgent = agent || (lead.agentId === user?.id ? { id: user.id, name: user.fullname || `${user.firstname || ''} ${user.lastname || ''}`.trim() || 'Você', role: user.role || 'Corretor', avatar: user.avatar_url || '' } : undefined);

 return (
 <tr
 key={lead.id}
 onClick={() => navigate(`/leads/${lead.id}`)}
 onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); navigate(`/leads/${lead.id}`); } }}
 tabIndex={0}
 role="link"
 aria-label={`Abrir detalhes do lead ${lead.name}`}
 className="hover:bg-muted/50 transition-colors group cursor-pointer focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1"
 >
 <td className="px-6 py-4 whitespace-nowrap">
 <div className="flex items-center gap-3">
 <Avatar className="w-10 h-10 border border-border">
 {lead.avatar ? <AvatarImage src={getAvatarThumbUrl(lead.avatar, 80)} alt={lead.name} /> : null}
 <AvatarFallback>{lead.name.charAt(0).toUpperCase()}</AvatarFallback>
 </Avatar>
 <div>
 <div className="font-semibold text-foreground group-hover:text-primary-600 transition-colors flex items-center gap-2">
 {lead.name}
 {(() => {
 const { label, icon: Icon, color } = getLeadTypeDetails(lead.leadType);
 return (
 <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-micro font-bold uppercase tracking-wider border ${color}`}>
 <Icon className="w-3 h-3 mr-1" />
 {label}
 </span>
 );
 })()}
 </div>
 <div className="text-xs text-muted-foreground mt-0.5">Ref: {lead.displayId || lead.id.substring(0, 8)}</div>
 {lead.sharedFrom && (
 <div className="mt-1">
 <SharedBadge companyName={lead.sharedFrom.companyName} />
 </div>
 )}
 </div>
 </div>
 </td>
 <td className="px-6 py-4 whitespace-nowrap">
 {assignedAgent ? (
 <div className="flex items-center gap-2">
 <div onClick={(e) => e.stopPropagation()}>
 <LeadAssigneePicker agents={agents} value={lead.agentId || ""} disabled={!canAssign} onChange={(next) => onAssignLead(lead.id, next)} />
 </div>
 <span className="text-sm text-foreground font-medium">{assignedAgent.name}</span>
 </div>
 ) : (
 <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
 <LeadAssigneePicker agents={agents} value={lead.agentId || ""} disabled={!canAssign} onChange={(next) => onAssignLead(lead.id, next)} />
 <span className="text-xs text-muted-foreground italic">Não atribuído</span>
 </div>
 )}
 </td>
 <td className="px-6 py-4 whitespace-nowrap">
 <div className="flex flex-col gap-1.5" onClick={(e) => e.stopPropagation()}>
 <div className="flex items-center text-sm text-muted-foreground hover:text-primary-600 cursor-pointer">
 <Mail className="w-3.5 h-3.5 mr-2 text-muted-foreground" />
 {lead.email}
 </div>
 <div className="flex items-center text-sm text-muted-foreground hover:text-primary-600 cursor-pointer">
 <Phone className="w-3.5 h-3.5 mr-2 text-muted-foreground" />
 {formatPhoneBR(lead.phone)}
 </div>
 </div>
 </td>
 <td className="px-6 py-4 whitespace-nowrap">
 <div className="flex items-center text-sm text-foreground font-medium">
 {resolvePropertyDisplay(lead.property, properties)}
 <ExternalLink className="w-3.5 h-3.5 ml-2 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer hover:text-primary-600" />
 </div>
 </td>
 <td className="px-6 py-4 whitespace-nowrap">
 <div className="text-sm font-semibold text-foreground">
 {formattedBudget}
 </div>
 </td>
 <td className="px-6 py-4 whitespace-nowrap">
 <span className={`inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium border
 ${lead.status === 'Novo' ? 'bg-primary-50 text-primary-700 border-primary-200' :
 lead.status === 'Em Atendimento' ? 'bg-primary-50 text-primary-700 border-primary-200' :
 lead.status === 'Qualificado' ? 'bg-cyan-50 text-cyan-700 border-cyan-200' :
 lead.status === 'Visita Agendada' ? 'bg-stage-contract/10 text-stage-contract border-stage-contract/30' :
 lead.status === 'Em Negociação' ? 'bg-status-warning-soft text-status-warning border-status-warning/30' :
 lead.status === 'Proposta Enviada' ? 'bg-orange-50 text-orange-700 border-orange-200' :
 lead.status === 'Convertido' ? 'bg-status-ok-soft text-status-ok border-status-ok/30' :
 'bg-status-critical-soft text-status-critical border-status-critical/30'
 }
 `}>
 <span className={`w-1.5 h-1.5 rounded-full mr-1.5
 ${lead.status === 'Novo' ? 'bg-primary-500' :
 lead.status === 'Em Atendimento' ? 'bg-primary-500' :
 lead.status === 'Qualificado' ? 'bg-cyan-500' :
 lead.status === 'Visita Agendada' ? 'bg-stage-contract' :
 lead.status === 'Em Negociação' ? 'bg-status-warning' :
 lead.status === 'Proposta Enviada' ? 'bg-orange-500' :
 lead.status === 'Convertido' ? 'bg-status-ok' :
 'bg-status-critical'
 }
 `}></span>
 {lead.status}
 </span>
 </td>
 <td className="px-6 py-4 whitespace-nowrap">
 <div className="flex items-center text-sm text-muted-foreground">
 <Calendar className="w-4 h-4 mr-2 text-muted-foreground" />
 {formattedDate}
 </div>
 </td>
 <td className="px-6 py-4 whitespace-nowrap text-right">
 <Button
 type="button"
 variant="ghost"
 size="icon"
 onClick={(e) => { e.stopPropagation(); }}
 aria-label="Mais ações"
 className="size-9 text-muted-foreground"
 >
 <MoreVertical className="w-5 h-5" aria-hidden="true" />
 </Button>
 </td>
 </tr>
 );
 })}
 </tbody>
 </table>
 </div>
 
 <div className="mt-auto px-6 py-4 border-t border-border bg-muted flex items-center justify-between">
 <span className="text-sm text-muted-foreground">
 Mostrando <span className="font-medium text-foreground">1</span> até <span className="font-medium text-foreground">{leads.length}</span> de <span className="font-medium text-foreground">{leads.length}</span> leads
 </span>
 <div className="flex gap-2">
 <Button variant="outline" size="sm" disabled>Anterior</Button>
 <Button variant="outline" size="sm">Próxima</Button>
 </div>
 </div>
 </div>
 );
}
