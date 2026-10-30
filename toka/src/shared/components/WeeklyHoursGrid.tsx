import { Input } from "../ui/input";
import { Checkbox } from "../ui/checkbox";

type TimeWindow = { start: string; end: string };
export type BusinessHoursValue = { timezone?: string; windows?: Record<string, TimeWindow> } | null;

const DAYS: Array<{ key: string; label: string }> = [
  { key: "0", label: "Dom" },
  { key: "1", label: "Seg" },
  { key: "2", label: "Ter" },
  { key: "3", label: "Qua" },
  { key: "4", label: "Qui" },
  { key: "5", label: "Sex" },
  { key: "6", label: "Sáb" },
];

export function WeeklyHoursGrid(props: {
  value: BusinessHoursValue;
  onChange: (next: BusinessHoursValue) => void;
  timezone?: string;
}) {
  const windows = props.value?.windows ?? {};
  const timezone = props.value?.timezone ?? props.timezone ?? "America/Sao_Paulo";

  const setDayEnabled = (dayKey: string, enabled: boolean) => {
    const nextWindows = { ...windows };
    if (!enabled) {
      delete nextWindows[dayKey];
    } else {
      nextWindows[dayKey] = nextWindows[dayKey] ?? { start: "09:00", end: "18:00" };
    }
    props.onChange({ timezone, windows: nextWindows });
  };

  const setWindow = (dayKey: string, patch: Partial<TimeWindow>) => {
    const curr = windows[dayKey] ?? { start: "09:00", end: "18:00" };
    const nextWindows = { ...windows, [dayKey]: { ...curr, ...patch } };
    props.onChange({ timezone, windows: nextWindows });
  };

  return (
    <div className="space-y-2">
      <div className="text-xs text-muted-foreground">Timezone: {timezone}</div>
      <div className="grid grid-cols-1 gap-2">
        {DAYS.map((d) => {
          const enabled = !!windows[d.key];
          const w = windows[d.key] ?? { start: "09:00", end: "18:00" };
          return (
            <div key={d.key} className="grid grid-cols-[84px_1fr_1fr] items-center gap-2">
              <label className="flex items-center gap-2 text-xs font-bold text-foreground">
                <Checkbox checked={enabled} onCheckedChange={(v) => setDayEnabled(d.key, v === true)} />
                {d.label}
              </label>
              <Input
                type="time"
                value={enabled ? w.start : ""}
                onChange={(e) => setWindow(d.key, { start: e.target.value })}
                disabled={!enabled}
                aria-label={`Início ${d.label}`}
              />
              <Input
                type="time"
                value={enabled ? w.end : ""}
                onChange={(e) => setWindow(d.key, { end: e.target.value })}
                disabled={!enabled}
                aria-label={`Fim ${d.label}`}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
