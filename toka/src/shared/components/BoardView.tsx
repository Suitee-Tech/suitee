import { useMemo } from "react";
import { useDrag, useDrop } from "react-dnd";
import { motion, AnimatePresence } from "motion/react";
import { useNavigate } from "react-router";
import {
 MoreHorizontal,
 MessageCircle,
 Phone,
 Calendar,
 Wallet,
 Building2,
 Clock,
 Edit2,
 Mail,
 User,
 Loader2,
} from "lucide-react";
import { Lead, LeadStatus, resolvePropertyDisplay } from "../data";
import type { Agent } from "../types/agent";
import { usePropertiesContext } from "../context/PropertiesContext";
import { Avatar, AvatarImage, AvatarFallback } from "../ui/avatar";
import { Button } from "../ui/button";
import { getAvatarThumbUrl } from "../media";
import { LeadAssigneePicker } from "./LeadAssigneePicker";
import { formatBRL } from "../lib/leads/format";
import { getLeadTypeDetails } from "../lib/leads/labels";
import { SharedBadge } from "./SharedBadge";

interface BoardViewProps {
 leads: Lead[];
 onMoveLead: (id: string, newStatus: LeadStatus) => void;
 agents: Agent[];
 user?: any;
 canAssign?: boolean;
 onAssignLead: (leadId: string, agentId: string) => void;
 movingLeadIds?: Set<string>;
}

const ItemTypes = {
 CARD: "card",
};

// Lead funnel column colors. Universal outcomes (Convertido, Perdido) use the
// shared status tokens; intermediate stages keep distinctive colors so the
// funnel remains scannable. The intermediate ones still pull from Tailwind's
// raw palette as they're not fundamentally semantic — they're identity hues
// for the lead funnel and don't need to vary by tenant.
const COLUMNS: { id: LeadStatus; label: string; color: string; border: string; bg: string }[] = [
 { id: "Novo", label: "Novo Lead", color: "text-primary-700", bg: "bg-primary-100", border: "border-primary-200" },
 { id: "Em Atendimento", label: "Em Atendimento", color: "text-primary-700", bg: "bg-primary-100", border: "border-primary-200" },
 { id: "Qualificado", label: "Qualificado", color: "text-status-info", bg: "bg-status-info-soft", border: "border-status-info/20" },
 { id: "Visita Agendada", label: "Visita Agendada", color: "text-stage-contract", bg: "bg-stage-contract/10", border: "border-stage-contract/20" },
 { id: "Em Negociação", label: "Em Negociação", color: "text-status-warning", bg: "bg-status-warning-soft", border: "border-status-warning/20" },
 { id: "Proposta Enviada", label: "Proposta Enviada", color: "text-stage-analysis", bg: "bg-stage-analysis/10", border: "border-stage-analysis/20" },
 { id: "Convertido", label: "Convertido", color: "text-status-ok", bg: "bg-status-ok-soft", border: "border-status-ok/20" },
 { id: "Perdido", label: "Perdido", color: "text-status-critical", bg: "bg-status-critical-soft", border: "border-status-critical/20" },
];

export function BoardView({ leads, onMoveLead, agents, user, canAssign, onAssignLead, movingLeadIds }: BoardViewProps) {
 const leadsByStatus = useMemo(() => {
  const grouped: Record<string, Lead[]> = {};
  for (const c of COLUMNS) grouped[c.id] = [];
  // Guard against a transient duplicate id in `leads` (e.g. a real-time upsert
  // racing a local insert): render each lead once so framer-motion's
  // layoutId={lead.id} never renders two cards sharing an id.
  const seen = new Set<string>();
  for (const l of leads) {
   if (seen.has(l.id)) continue;
   seen.add(l.id);
   const k = l.status;
   if (!grouped[k]) continue;
   grouped[k].push(l);
  }
  return grouped;
 }, [leads]);

 const agentById = useMemo(() => {
  const m: Record<string, Agent> = {};
  for (const a of agents) {
   if (a?.id) m[a.id] = a;
  }
  return m;
 }, [agents]);

 return (
 <div className="flex gap-6 h-full overflow-x-auto pb-4 custom-scrollbar">
 {COLUMNS.map((col) => (
 <Column 
      key={col.id} 
      column={col} 
      leads={leadsByStatus[col.id] || []} 
      onDropCard={(id: string) => onMoveLead(id, col.id)}
      user={user}
      agentById={agentById}
      agents={agents}
      canAssign={canAssign}
      onAssignLead={onAssignLead}
      movingLeadIds={movingLeadIds}
    />
 ))}
 </div>
 );
}

function Column({
 column,
 leads,
 onDropCard,
 user,
 agentById,
 agents,
 canAssign,
 onAssignLead,
 movingLeadIds,
}: {
 column: typeof COLUMNS[0];
 leads: Lead[];
 onDropCard: (id: string) => void;
 user?: any;
 agentById: Record<string, Agent>;
 agents: Agent[];
 canAssign?: boolean;
 onAssignLead: (leadId: string, agentId: string) => void;
 movingLeadIds?: Set<string>;
}) {
 const { properties } = usePropertiesContext();
 const [{ isOver, canDrop }, dropRef] = useDrop(() => ({
 accept: ItemTypes.CARD,
 canDrop: (item: { id: string; status: LeadStatus }) =>
  item.status !== column.id && !movingLeadIds?.has(item.id),
 drop: (item: { id: string; status: LeadStatus }, monitor) => {
  if (monitor.canDrop()) onDropCard(item.id);
 },
 collect: (monitor) => ({
 isOver: monitor.isOver({ shallow: true }),
 canDrop: monitor.canDrop(),
 }),
 }), [column.id, movingLeadIds, onDropCard]);

 const totalBudget = leads.reduce((acc: number, l: Lead) => acc + l.budget, 0);
 const formattedBudget = formatBRL(totalBudget);

 return (
 <section
 ref={dropRef as unknown as React.Ref<HTMLDivElement>}
 aria-label={`Coluna ${column.label}, ${leads.length} ${leads.length === 1 ? "lead" : "leads"}`}
 className={`flex flex-col flex-shrink-0 w-80 bg-muted/50 rounded-xl border-2 border-dashed transition-colors
 ${isOver && canDrop ? "border-primary-400 bg-primary-50/50" : "border-border bg-muted/50"}`}
 >
 <div className={`flex items-center justify-between p-4 border-b border-border/60 rounded-t-xl bg-muted`}>
 <div className="flex items-center gap-2">
 <span className={`w-2.5 h-2.5 rounded-full ${column.bg} ring-2 ring-card`} aria-hidden="true"></span>
 <h3 className="font-semibold text-foreground">{column.label}</h3>
 <span className="flex items-center justify-center bg-muted text-muted-foreground text-xs font-medium w-6 h-6 rounded-full ml-1" aria-hidden="true">
 {leads.length}
 </span>
 </div>
 <Button type="button" variant="ghost" size="icon" aria-label={`Mais ações na coluna ${column.label}`} className="size-8 text-muted-foreground">
 <MoreHorizontal className="w-5 h-5" aria-hidden="true" />
 </Button>
 </div>

 <div className="px-4 py-2 flex items-center justify-between text-xs text-muted-foreground font-medium border-b border-border/50 bg-muted/50">
 <span>Valor previsto:</span>
 <span className="text-foreground">{formattedBudget}</span>
 </div>

 <ul role="list" className="flex-1 overflow-y-auto p-4 space-y-4">
 <AnimatePresence>
 {leads.map((lead: Lead) => (
          <Card key={lead.id} lead={lead} column={column} user={user} agentById={agentById} agents={agents} canAssign={canAssign} onAssignLead={onAssignLead} isMoving={movingLeadIds?.has(lead.id)} />
        ))}
 </AnimatePresence>
 </ul>
 </section>
 );
}

function Card({
 lead,
 column,
 user,
 agentById,
 agents,
 canAssign,
 onAssignLead,
 isMoving,
}: {
 lead: Lead;
 column: typeof COLUMNS[0];
 user?: any;
 agentById: Record<string, Agent>;
 agents: Agent[];
 canAssign?: boolean;
 onAssignLead: (leadId: string, agentId: string) => void;
 isMoving?: boolean;
}) {
 const navigate = useNavigate();
 const { properties } = usePropertiesContext();
 const [{ isDragging }, dragRef] = useDrag(() => ({
 type: ItemTypes.CARD,
 item: { id: lead.id, status: lead.status },
 canDrag: !isMoving,
 collect: (monitor) => ({
 isDragging: !!monitor.isDragging(),
 }),
 }), [isMoving, lead.id, lead.status]);

 const formattedValue = formatBRL(lead.budget);

 const agent = lead.agentId ? agentById[lead.agentId] : undefined;
 const assignedAgent = agent || (lead.agentId === user?.id ? { id: user.id, name: user.fullname || `${user.firstname || ''} ${user.lastname || ''}`.trim() || 'Você', role: user.role || 'Corretor', avatar: user.avatar_url || '' } : undefined);
 const leadTypeDetails = getLeadTypeDetails(lead.leadType);
 const TypeIcon = leadTypeDetails.icon;

 return (
 <motion.li
 layout
 layoutId={lead.id}
 initial={{ opacity: 0, y: 10 }}
 animate={{ opacity: 1, y: 0 }}
 exit={{ opacity: 0, scale: 0.95 }}
 ref={dragRef as unknown as React.Ref<HTMLLIElement>}
 aria-label={`Card do lead ${lead.name}, status ${column.label}`}
 className={`group bg-card p-4 rounded-xl border shadow-sm cursor-grab active:cursor-grabbing hover:shadow-md transition-shadow list-none relative
 ${isDragging ? "opacity-40" : "opacity-100"}
 ${isMoving ? "opacity-70 pointer-events-none" : ""}
 ${column.border}`}
 >
 {isMoving && (
   <div className="absolute inset-0 bg-card/60 rounded-xl flex items-center justify-center z-10">
     <Loader2 className="w-5 h-5 text-primary animate-spin" />
   </div>
 )}
 <div 
 className="flex justify-between items-start mb-3 cursor-pointer"
 onClick={(e) => { e.stopPropagation(); navigate(`/leads/${lead.id}`); }}
 >
 <div className="flex items-center gap-3">
 <Avatar className="w-10 h-10 border border-border">
 {lead.avatar ? <AvatarImage src={getAvatarThumbUrl(lead.avatar, 80)} alt={lead.name} /> : null}
 <AvatarFallback>{lead.name.charAt(0).toUpperCase()}</AvatarFallback>
 </Avatar>
 <div>
 <h4 className="text-sm font-semibold text-foreground group-hover:text-primary-600 transition-colors flex items-center gap-2">
 {lead.name}
 </h4>
 <div className="flex flex-wrap items-center text-xs text-muted-foreground mt-0.5 gap-1.5">
 <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-caption font-bold uppercase tracking-wider border ${leadTypeDetails.color}`}>
 <TypeIcon className="w-2.5 h-2.5 mr-1" />
 {leadTypeDetails.label}
 </span>
 {lead.sharedFrom && <SharedBadge companyName={lead.sharedFrom.companyName} />}
 </div>
 </div>
 </div>
 </div>

 <div className="space-y-2 mb-4">
 <div className="flex items-center justify-between text-sm">
 <div className="flex items-center text-muted-foreground truncate max-w-[70%]">
 <Building2 className="w-4 h-4 mr-2 text-muted-foreground shrink-0" />
  <span className="truncate">{resolvePropertyDisplay(lead.property, properties)}</span>
 </div>
 <div className="flex items-center gap-1.5" title={assignedAgent ? `Atribuído a ${assignedAgent.name}` : "Sem responsável"}>
 <LeadAssigneePicker
 agents={agents}
 value={lead.agentId || ""}
 disabled={!canAssign}
 onChange={(next) => onAssignLead(lead.id, next)}
 />
 </div>
 </div>
 <div className="flex items-center text-sm font-medium text-foreground">
 <Wallet className="w-4 h-4 mr-2 text-primary-400" />
 <span>{formattedValue}</span>
 </div>
 </div>

 <div className="flex items-center justify-between pt-3 border-t border-border">
 <div className="flex items-center gap-1.5">
 <span className={`px-2 py-1 rounded text-caption font-bold uppercase tracking-wider ${column.bg} ${column.color}`}>
 {lead.status}
 </span>
 </div>
 
 <div className="flex gap-1">
 <Button
 variant="ghost"
 size="icon"
 title="WhatsApp"
 onClick={(e) => { e.stopPropagation(); window.open(`https://wa.me/${lead.phone.replace(/\D/g, '')}`, '_blank'); }}
 className="size-8 text-muted-foreground hover:text-[#25D366] hover:bg-[#25D366]/10"
 >
 <MessageCircle className="w-4 h-4" />
 </Button>
 <Button
 variant="ghost"
 size="icon"
 title="Enviar E-mail"
 onClick={(e) => { e.stopPropagation(); window.location.href = `mailto:${lead.email}`; }}
 className="size-8 text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
 >
 <Mail className="w-4 h-4" />
 </Button>
 <Button
 variant="ghost"
 size="icon"
 title="Editar Lead"
 onClick={(e) => { e.stopPropagation(); navigate(`/leads/${lead.id}`); }}
 className="size-8 text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
 >
 <Edit2 className="w-4 h-4" />
 </Button>
 </div>
 </div>
 </motion.li>
 );
}
