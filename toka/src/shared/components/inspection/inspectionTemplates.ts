export interface ChecklistItemTemplate {
  description: string;
}

export interface RoomTemplate {
  name: string;
  items: ChecklistItemTemplate[];
}

export const CONDITION_VALUES = ["Ótimo", "Bom", "Regular", "Ruim", "Danificado"] as const;
export type ConditionValue = (typeof CONDITION_VALUES)[number];

export const DEFAULT_RESIDENTIAL_TEMPLATE: RoomTemplate[] = [
  {
    name: "Sala",
    items: [
      { description: "Piso" },
      { description: "Paredes / Pintura" },
      { description: "Teto" },
      { description: "Janelas e esquadrias" },
      { description: "Tomadas e interruptores" },
    ],
  },
  {
    name: "Cozinha",
    items: [
      { description: "Pia e bancada" },
      { description: "Armários" },
      { description: "Azulejos" },
      { description: "Ralo e encanamento" },
      { description: "Exaustor / coifa" },
    ],
  },
  {
    name: "Quarto",
    items: [
      { description: "Piso" },
      { description: "Paredes / Pintura" },
      { description: "Janelas e esquadrias" },
      { description: "Closet / armários embutidos" },
    ],
  },
  {
    name: "Banheiro",
    items: [
      { description: "Pia e box" },
      { description: "Vaso sanitário" },
      { description: "Azulejos" },
      { description: "Chuveiro e torneiras" },
      { description: "Ralos e encanamento" },
    ],
  },
  {
    name: "Área Externa",
    items: [
      { description: "Varanda / sacada" },
      { description: "Piso externo" },
      { description: "Iluminação externa" },
    ],
  },
];

export function buildFindingsFromTemplate(template: RoomTemplate[]): Array<{
  id: string;
  room: string;
  description: string;
  condition: string;
  notes: string;
  photos: string[];
  thumbnails: string[];
}> {
  const out: Array<{
    id: string;
    room: string;
    description: string;
    condition: string;
    notes: string;
    photos: string[];
    thumbnails: string[];
  }> = [];
  for (const room of template) {
    for (const item of room.items) {
      out.push({
        id: makeFindingId(),
        room: room.name,
        description: item.description,
        condition: "",
        notes: "",
        photos: [],
        thumbnails: [],
      });
    }
  }
  return out;
}

export function makeFindingId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `f-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

export async function loadInspectionTemplate(): Promise<RoomTemplate[]> {
  try {
    const { default: api } = await import("../../services/api");
    const response = await api.get<{ data: RoomTemplate[] }>("/inspections/template");
    const tpl = response.data?.data;
    if (Array.isArray(tpl) && tpl.length > 0) {
      return tpl;
    }
  } catch (err) {
    if (typeof console !== "undefined") {
      console.warn("[inspectionTemplates] failed to load tenant template, using local default:", err);
    }
  }
  return DEFAULT_RESIDENTIAL_TEMPLATE;
}
