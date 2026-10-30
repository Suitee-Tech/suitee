import { Clock } from "lucide-react";
import { Button } from "../../../ui/button";
import { WeeklyHoursGrid, type BusinessHoursValue } from "../../../components/WeeklyHoursGrid";
import { SectionCard, SettingsInput } from "../_shared";

interface Props {
  horarioAbertura: string;
  horarioFechamento: string;
  businessHours: BusinessHoursValue;
  onChangeHorarioAbertura: (v: string) => void;
  onChangeHorarioFechamento: (v: string) => void;
  onChangeBusinessHours: (v: BusinessHoursValue) => void;
}

export function BusinessHoursSection({
  horarioAbertura,
  horarioFechamento,
  businessHours,
  onChangeHorarioAbertura,
  onChangeHorarioFechamento,
  onChangeBusinessHours,
}: Props) {
  return (
    <SectionCard title="Horário de Funcionamento" subtitle="Defina os dias e horários de operação da equipe" delay={0.1}>
      <div className="space-y-5">
        <div className="grid grid-cols-1 lg:grid-cols-[320px_1fr] gap-6 items-start">
          <div className="space-y-1.5">
            <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2 block">Horário padrão</label>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Abertura</label>
                <SettingsInput value={horarioAbertura} onChange={onChangeHorarioAbertura} type="time" icon={<Clock className="w-4 h-4" />} />
              </div>
              <div className="space-y-1.5">
                <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Fechamento</label>
                <SettingsInput value={horarioFechamento} onChange={onChangeHorarioFechamento} type="time" icon={<Clock className="w-4 h-4" />} />
              </div>
            </div>
            <div className="flex flex-wrap gap-2 pt-3">
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  const windows: Record<string, { start: string; end: string }> = {};
                  ["1", "2", "3", "4", "5"].forEach((k) => {
                    windows[k] = { start: horarioAbertura, end: horarioFechamento };
                  });
                  onChangeBusinessHours({ timezone: "America/Sao_Paulo", windows });
                }}
              >
                Aplicar Seg–Sex
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  const windows: Record<string, { start: string; end: string }> = {};
                  ["0", "1", "2", "3", "4", "5", "6"].forEach((k) => {
                    windows[k] = { start: horarioAbertura, end: horarioFechamento };
                  });
                  onChangeBusinessHours({ timezone: "America/Sao_Paulo", windows });
                }}
              >
                Aplicar Todos
              </Button>
              <Button type="button" variant="outline" onClick={() => onChangeBusinessHours({ timezone: "America/Sao_Paulo", windows: {} })}>
                Limpar
              </Button>
            </div>
          </div>
          <div className="rounded-2xl border border-border bg-muted/30 p-4">
            <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-3 block">Horário por dia</label>
            <WeeklyHoursGrid value={businessHours} onChange={onChangeBusinessHours} />
          </div>
        </div>
      </div>
    </SectionCard>
  );
}
