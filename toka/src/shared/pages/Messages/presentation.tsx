import {
  Briefcase,
  Building2,
  FileText,
  Handshake,
  Hash,
  MessageSquare,
  Shield,
  UserCircle,
  Users,
} from "lucide-react";
import type { ChannelType } from "../../services/message.service";

export const CHANNEL_TYPE_META: Record<ChannelType, { icon: typeof Hash; label: string; color: string }> = {
  direct: { icon: MessageSquare, label: "Direto", color: "text-status-info bg-status-info-soft" },
  group: { icon: Users, label: "Grupo", color: "text-violet-600 bg-violet-50 dark:bg-violet-900/30 dark:text-violet-400" },
  support: { icon: Shield, label: "Suporte", color: "text-status-critical bg-status-critical-soft" },
  lead: { icon: UserCircle, label: "Lead", color: "text-status-warning bg-status-warning-soft" },
  deal: { icon: Briefcase, label: "Negócio", color: "text-status-ok bg-status-ok-soft" },
  property: { icon: Building2, label: "Imóvel", color: "text-sky-600 bg-sky-50 dark:bg-sky-900/30 dark:text-sky-400" },
  contract: { icon: FileText, label: "Contrato", color: "text-primary-600 bg-primary-50 dark:bg-primary-900/30 dark:text-primary-400" },
  team: { icon: Hash, label: "Equipe", color: "text-teal-600 bg-teal-50 dark:bg-teal-900/30 dark:text-teal-400" },
  partner: { icon: Handshake, label: "Parceiro", color: "text-orange-600 bg-orange-50 dark:bg-orange-900/30 dark:text-orange-400" },
};

export const EMOJI_CATEGORIES = [
  { label: "Frequentes", emojis: ["👍", "❤️", "😂", "😮", "😢", "🙏", "🔥", "✅"] },
  {
    label: "Rostos",
    emojis: [
      "😀",
      "😃",
      "😄",
      "😁",
      "😆",
      "😅",
      "🤣",
      "😂",
      "🙂",
      "😉",
      "😊",
      "😇",
      "🥰",
      "😍",
      "🤩",
      "😘",
      "😗",
      "😚",
      "😋",
      "😛",
      "😜",
      "🤪",
      "😝",
      "🤑",
      "🤗",
      "🤭",
      "🤔",
      "🤐",
      "🤨",
      "😐",
      "😑",
      "😶",
      "😏",
      "😒",
      "🙄",
      "😬",
      "😮‍💨",
      "🤥",
      "😌",
      "😔",
      "😪",
      "🤤",
      "😴",
      "😷",
      "🤒",
      "🤕",
      "🤢",
      "🤮",
      "🤧",
      "🥵",
      "🥶",
      "🥴",
      "😵",
      "🤯",
      "😎",
      "🥸",
      "🤠",
      "😤",
      "😡",
      "🤬",
      "😈",
    ],
  },
  {
    label: "Gestos",
    emojis: [
      "👋",
      "🤚",
      "🖐️",
      "✋",
      "🖖",
      "👌",
      "🤌",
      "🤏",
      "✌️",
      "🤞",
      "🤟",
      "🤘",
      "🤙",
      "👈",
      "👉",
      "👆",
      "👇",
      "☝️",
      "👍",
      "👎",
      "✊",
      "👊",
      "🤛",
      "🤜",
      "👏",
      "🙌",
      "👐",
      "🤲",
      "🤝",
      "🙏",
      "💪",
    ],
  },
  {
    label: "Objetos",
    emojis: [
      "📎",
      "📁",
      "📂",
      "📄",
      "📃",
      "📋",
      "📌",
      "📍",
      "🔑",
      "🏠",
      "🏢",
      "🏗️",
      "💰",
      "💵",
      "💳",
      "📊",
      "📈",
      "📉",
      "📅",
      "📆",
      "🗓️",
      "📞",
      "📧",
      "💻",
      "🖨️",
      "⚖️",
      "🔒",
      "🔓",
    ],
  },
  {
    label: "Símbolos",
    emojis: [
      "✅",
      "❌",
      "⚠️",
      "🚫",
      "❗",
      "❓",
      "💯",
      "🔥",
      "✨",
      "⭐",
      "🎉",
      "🎊",
      "🏆",
      "🥇",
      "💎",
      "🔔",
      "🔕",
      "📢",
      "💬",
      "💭",
      "🗨️",
      "♻️",
      "🆕",
      "🆗",
      "🔄",
      "⏰",
    ],
  },
];

export const QUICK_REACTIONS = ["👍", "❤️", "😂", "😮", "😢", "🔥"];

export function formatMessageTime(iso: string): string {
  return new Date(iso).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });
}

export function formatChannelTime(iso: string): string {
  const date = new Date(iso);
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const days = Math.floor(diff / 86400000);
  if (days === 0) return date.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });
  if (days === 1) return "Ontem";
  if (days < 7) return date.toLocaleDateString("pt-BR", { weekday: "short" });
  return date.toLocaleDateString("pt-BR", { day: "2-digit", month: "2-digit" });
}

export function formatDateSeparator(iso: string): string {
  const date = new Date(iso);
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const days = Math.floor(diff / 86400000);
  if (days === 0) return "Hoje";
  if (days === 1) return "Ontem";
  return date.toLocaleDateString("pt-BR", { day: "2-digit", month: "long", year: "numeric" });
}

export function getInitials(name: string): string {
  return name
    .split(" ")
    .map((w) => w[0])
    .filter(Boolean)
    .slice(0, 2)
    .join("")
    .toUpperCase();
}

export function highlightText(text: string, highlight: string) {
  if (!highlight.trim()) return <span>{text}</span>;
  const escapedHighlight = highlight.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const regex = new RegExp(`(${escapedHighlight})`, "gi");
  const parts = text.split(regex);
  return (
    <span>
      {parts.map((part, i) =>
        part.localeCompare(highlight, "pt-BR", { sensitivity: "accent" }) === 0 ? (
          <mark key={i} className="bg-yellow-200 dark:bg-yellow-800/50 text-foreground dark:text-yellow-100 rounded px-0.5">
            {part}
          </mark>
        ) : (
          <span key={i}>{part}</span>
        ),
      )}
    </span>
  );
}

export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1048576).toFixed(1)} MB`;
}

const AVATAR_COLORS = [
  "bg-primary-500",
  "bg-status-ok",
  "bg-status-warning",
  "bg-status-critical",
  "bg-sky-500",
  "bg-violet-500",
  "bg-teal-500",
  "bg-orange-500",
  "bg-pink-500",
  "bg-cyan-500",
];

export function avatarColor(id: string): string {
  let hash = 0;
  for (let i = 0; i < id.length; i++) hash = id.charCodeAt(i) + ((hash << 5) - hash);
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length];
}
