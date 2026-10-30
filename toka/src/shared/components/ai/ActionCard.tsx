import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "../../ui/card";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";
import { Textarea } from "../../ui/textarea";
import { Switch } from "../../ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../../ui/select";
import { useForm } from "react-hook-form";
import { useMemo, useState } from "react";

export type SuiteeAction = {
  id?: string;
  action_id?: string;
  tool_name?: string;
  title: string;
  description?: string;
  primary_label?: string;
  secondary_label?: string;
  confirm_message?: string;
  prefill?: string;
  inputs_schema?: any;
  inputs_default?: Record<string, any>;
};

type Props = {
  action: SuiteeAction;
  onPrimary: (action: SuiteeAction, inputs: Record<string, any>) => void;
  onSecondary?: (action: SuiteeAction) => void;
  isLoading?: boolean;
};

const INPUT_LABELS: Record<string, string> = {
  date: "Data",
  start_date: "Data inicial",
  end_date: "Data final",
  due_date: "Vencimento",
  owner_id: "Proprietário",
  tenant_id: "Inquilino",
  property_id: "Imóvel",
  contract_id: "Contrato",
  amount: "Valor",
  value: "Valor",
  status: "Status",
  reason: "Motivo",
  note: "Observação",
};

const labelForInput = (key: string, prop: unknown) => {
  const schemaLabel = (prop as { title?: unknown; description?: unknown } | null)?.title;
  if (typeof schemaLabel === "string" && schemaLabel.trim()) return schemaLabel;
  return INPUT_LABELS[key] ?? key.replace(/_/g, " ");
};

export function ActionCard({ action, onPrimary, onSecondary, isLoading }: Props) {
  const [showRaw, setShowRaw] = useState(false);
  const schema = action.inputs_schema;
  const defaultValues = useMemo(() => action.inputs_default || {}, [action.inputs_default]);
  const form = useForm<Record<string, any>>({ defaultValues });

  const properties: Record<string, any> = useMemo(() => {
    if (!schema || typeof schema !== "object") return {};
    const props = (schema as any).properties;
    if (!props || typeof props !== "object") return {};
    return props as Record<string, any>;
  }, [schema]);

  const hasSupportedSchema = useMemo(() => {
    if (!schema || typeof schema !== "object") return false;
    if ((schema as any).type !== "object") return false;
    return Object.keys(properties).length > 0;
  }, [schema, properties]);

  const submit = form.handleSubmit((values) => onPrimary(action, values || {}));

  return (
    <Card className="border-primary/20 bg-primary/[0.03]">
      <CardHeader className="pb-3">
        <CardTitle className="text-sm">{action.title}</CardTitle>
      </CardHeader>
      {(action.description || "").trim() !== "" && (
        <CardContent className="pt-0 text-sm text-muted-foreground">{action.description}</CardContent>
      )}
      {hasSupportedSchema && (
        <CardContent className="pt-0">
          <form onSubmit={submit} className="space-y-3">
            {!showRaw &&
              Object.entries(properties).map(([key, prop]) => {
                const type = (prop as any)?.type;
                const enumValues: string[] | undefined = Array.isArray((prop as any)?.enum) ? (prop as any).enum : undefined;
                if (type === "string" && enumValues && enumValues.length > 0) {
                  return (
                    <div key={key} className="space-y-1">
                      <div className="text-xs text-muted-foreground">{labelForInput(key, prop)}</div>
                      <Select value={String(form.watch(key) ?? "")} onValueChange={(v) => form.setValue(key, v, { shouldDirty: true })}>
                        <SelectTrigger className="h-9">
                          <SelectValue placeholder="Selecione" />
                        </SelectTrigger>
                        <SelectContent>
                          {enumValues.map((v) => (
                            <SelectItem key={v} value={v}>
                              {v}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  );
                }
                if (type === "boolean") {
                  return (
                    <div key={key} className="flex items-center justify-between gap-3">
                      <div className="text-xs text-muted-foreground">{labelForInput(key, prop)}</div>
                      <Switch checked={Boolean(form.watch(key))} onCheckedChange={(v) => form.setValue(key, v, { shouldDirty: true })} />
                    </div>
                  );
                }
                if (type === "number" || type === "integer") {
                  return (
                    <div key={key} className="space-y-1">
                      <div className="text-xs text-muted-foreground">{labelForInput(key, prop)}</div>
                      <Input
                        type="number"
                        value={form.watch(key) ?? ""}
                        onChange={(e) =>
                          form.setValue(key, e.target.value === "" ? undefined : Number(e.target.value), { shouldDirty: true })
                        }
                        className="h-9"
                      />
                    </div>
                  );
                }
                if (type === "string" || type === undefined) {
                  return (
                    <div key={key} className="space-y-1">
                      <div className="text-xs text-muted-foreground">{labelForInput(key, prop)}</div>
                      <Input
                        value={form.watch(key) ?? ""}
                        onChange={(e) => form.setValue(key, e.target.value, { shouldDirty: true })}
                        className="h-9"
                      />
                    </div>
                  );
                }
                return (
                  <div key={key} className="space-y-1">
                    <div className="text-xs text-muted-foreground">{labelForInput(key, prop)}</div>
                    <Textarea
                      value={JSON.stringify(form.watch(key) ?? null, null, 2)}
                      onChange={(e) => {
                        try {
                          form.setValue(key, JSON.parse(e.target.value), { shouldDirty: true });
                        } catch {
                          form.setValue(key, e.target.value, { shouldDirty: true });
                        }
                      }}
                      className="min-h-[80px]"
                    />
                  </div>
                );
              })}
            {showRaw && (
              <Textarea
                value={JSON.stringify(form.getValues() ?? {}, null, 2)}
                onChange={(e) => {
                  try {
                    form.reset(JSON.parse(e.target.value));
                  } catch {
                  }
                }}
                className="min-h-[140px] font-mono text-xs"
              />
            )}
            <div className="flex gap-2">
              <Button type="submit" size="sm" disabled={Boolean(isLoading)}>
                {action.primary_label || "Executar"}
              </Button>
              <Button type="button" size="sm" variant="outline" onClick={() => setShowRaw((v) => !v)}>
                {showRaw ? "Campos" : action.secondary_label || "Editar"}
              </Button>
            </div>
          </form>
        </CardContent>
      )}
      {!hasSupportedSchema && (
        <CardFooter className="gap-2">
          <Button onClick={() => onPrimary(action, {})} size="sm" disabled={Boolean(isLoading)}>
            {action.primary_label || "Executar"}
          </Button>
          {onSecondary && (
            <Button onClick={() => onSecondary(action)} size="sm" variant="outline">
              {action.secondary_label || "Editar"}
            </Button>
          )}
        </CardFooter>
      )}
    </Card>
  );
}

