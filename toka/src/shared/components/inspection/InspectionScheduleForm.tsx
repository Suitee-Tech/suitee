import { useCallback, useId, useState } from "react";
import { toast } from "sonner";
import { Button } from "../../ui/button";
import { SupplierPicker } from "../suppliers/SupplierPicker";
import { inspectionService, type Inspection } from "../../services/inspection.service";
import type { Supplier } from "../../services/supplier.service";
import type { Contract } from "../../contracts/types";

export function InspectionScheduleForm({ contract, inspection, onSaved, onCancel }: {
  inspection?: Inspection;
  contract: Contract;
  onSaved: () => void;
  onCancel: () => void;
}) {
  const id = useId();
  const [type, setType] = useState(inspection?.type ?? "ENTRY");
  const [date, setDate] = useState(() => {
    if (!inspection) return "";
    const value = new Date(inspection.scheduled_date);
    return new Date(value.getTime() - value.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
  });
  const [supplierId, setSupplierId] = useState(inspection?.inspector_id ?? "");
  const [supplier, setSupplier] = useState<Supplier>();
  const [notes, setNotes] = useState(inspection?.notes ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const selectSupplier = useCallback((value: Supplier | undefined) => setSupplier(value), []);
  const submit = async () => {
    if (saving) return;
    if (!supplier?.active || supplier.id !== supplierId || !date || !contract.property_id) {
      setError("Selecione um vistoriador, informe data e horário e vincule um imóvel ao contrato.");
      return;
    }
    setSaving(true);
    setError("");
    try {
      const payload = { property_id: contract.property_id, contract_id: contract.id,
        time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone, type, scheduled_date: new Date(date).toISOString(), inspector_id: supplier.id,
        inspector_name: supplier.name, notes };
      if (inspection) await inspectionService.update(inspection.id, payload);
      else await inspectionService.create(payload);
      toast.success(inspection ? "Agendamento atualizado. A agenda será sincronizada." : "Vistoria agendada. O compromisso será sincronizado com a agenda.");
      onSaved();
    } catch {
      setError("Não foi possível agendar a vistoria. Seus dados foram mantidos; tente novamente.");
    } finally { setSaving(false); }
  };
  return <div className="space-y-4 p-5 border-b border-border">
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
      <div><label htmlFor={`${id}-type`} className="block text-label font-bold mb-1">Tipo</label>
        <select id={`${id}-type`} value={type} onChange={(e) => setType(e.target.value)} className="w-full rounded-lg border border-border bg-card p-2">
          <option value="ENTRY">Entrada</option><option value="EXIT">Saída</option><option value="PERIODIC">Periódica</option>
        </select></div>
      <div><label htmlFor={`${id}-date`} className="block text-label font-bold mb-1">Data e horário</label>
        <input id={`${id}-date`} type="datetime-local" value={date} onChange={(e) => setDate(e.target.value)} className="w-full rounded-lg border border-border bg-card p-2" /></div>
    </div>
    <SupplierPicker value={supplierId} onChange={setSupplierId} onSupplierSelected={selectSupplier} label="Vistoriador" required inspectorsOnly />
    <div><label htmlFor={`${id}-notes`} className="block text-label font-bold mb-1">Observações</label>
      <textarea id={`${id}-notes`} value={notes} onChange={(e) => setNotes(e.target.value)} className="w-full rounded-lg border border-border bg-card p-2" /></div>
    <p className="text-caption text-muted-foreground">O horário usa o fuso deste dispositivo. A vistoria aparecerá na sua agenda e na visão da equipe.</p>
    {error && <p role="alert" className="text-label text-status-critical">{error}</p>}
    <div className="flex justify-end gap-2"><Button variant="ghost" onClick={onCancel} disabled={saving}>Cancelar</Button><Button onClick={submit} loading={saving}>{inspection ? "Salvar agendamento" : "Agendar vistoria"}</Button></div>
  </div>;
}
