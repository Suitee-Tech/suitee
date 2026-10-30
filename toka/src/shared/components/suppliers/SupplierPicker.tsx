import { useCallback, useEffect, useId, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Briefcase, Loader2, Pencil, Phone, Plus, UserRoundCheck } from "lucide-react";
import { toast } from "sonner";

import { collectPages } from "../../services/collectPages";
import { Button } from "../../ui/button";
import { Input, inputClassNames } from "../../ui/input";
import { PhoneInput } from "../../ui/phone-input";
import { DocumentInput } from "../../ui/document-input";
import { isValidCpfCnpj, parsePhoneBR, parseCpfCnpj } from "../../utils/masks";
import { zEmail } from "../../validation/fields";
import { SearchableSelect, type SearchableSelectOption } from "../../ui/searchable-select";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "../../ui/sheet";
import {
  isInspectorService,
  supplierContact,
  supplierDisplayName,
  supplierService,
  type CreateSupplierDTO,
  type Supplier,
  type SupplierKind,
} from "../../services/supplier.service";

const SUPPLIER_QUERY_KEY = "suppliers";

/**
 * Campos do cadastro de prestador. Reaproveitado pela seleção inline e pelo
 * formulário de edição — as características, o contato e o tipo de serviço
 * ficam no cadastro, para o próximo chamado só escolher quem já atende.
 */
interface SupplierFormState {
  name: string;
  serviceType: string;
  kind: SupplierKind;
  phone: string;
  email: string;
  document: string;
  city: string;
  notes: string;
}

const EMPTY_FORM: SupplierFormState = {
  name: "",
  serviceType: "",
  kind: "externo",
  phone: "",
  email: "",
  document: "",
  city: "",
  notes: "",
};

function formFromSupplier(supplier: Supplier): SupplierFormState {
  return {
    name: supplier.name,
    serviceType: supplier.service_type,
    kind: supplier.kind ?? "externo",
    phone: supplier.phone ?? "",
    email: supplier.email ?? "",
    document: supplier.document ?? "",
    city: supplier.city ?? "",
    notes: supplier.notes ?? "",
  };
}

function toDTO(form: SupplierFormState): CreateSupplierDTO {
  return {
    name: form.name.trim(),
    service_type: form.serviceType.trim(),
    kind: form.kind,
    phone: parsePhoneBR(form.phone),
    email: form.email.trim().toLowerCase(),
    document: parseCpfCnpj(form.document),
    city: form.city.trim(),
    notes: form.notes.trim(),
  };
}

/**
 * Validação local espelhando a do serviço: sem tipo de serviço o prestador não
 * é reutilizável e sem contato ninguém consegue acioná-lo.
 */
const validAreaCodes = new Set("11 12 13 14 15 16 17 18 19 21 22 24 27 28 31 32 33 34 35 37 38 41 42 43 44 45 46 47 48 49 51 53 54 55 61 62 63 64 65 66 67 68 69 71 73 74 75 77 79 81 82 83 84 85 86 87 88 89 91 92 93 94 95 96 97 98 99".split(" "));
type SupplierFormErrors = Partial<Record<keyof SupplierFormState, string>>;
export function supplierFormErrors(form: SupplierFormState): SupplierFormErrors {
  const errors: SupplierFormErrors = {};
  if (!form.name.trim()) errors.name = "Informe o nome do profissional.";
  if (!form.serviceType.trim()) errors.serviceType = "Informe o tipo de serviço.";
  if (!form.phone.trim() && !form.email.trim()) {
    errors.phone = "Informe telefone ou e-mail de contato.";
    errors.email = errors.phone;
  }
  if (form.phone.trim()) {
    let phone = form.phone.replace(/\D/g, "");
    if (phone.length > 11 && phone.startsWith("55")) phone = phone.slice(2);
    if (!validAreaCodes.has(phone.slice(0, 2)) || !/^(?:\d{2}[2-5]\d{7}|\d{2}9\d{8})$/.test(phone)) {
      errors.phone = "Informe um telefone válido com DDD.";
    }
  }
  if (form.email.trim() && !zEmail().safeParse(form.email.trim()).success) errors.email = "Informe um e-mail válido.";
  if (form.document.trim() && !isValidCpfCnpj(form.document)) errors.document = "Informe um CPF ou CNPJ válido.";
  return errors;
}
export function validateSupplierForm(form: SupplierFormState): string | null {
  return Object.values(supplierFormErrors(form))[0] ?? null;
}

export function SupplierFields({
  form,
  onChange,
  serviceTypeOptions,
  idPrefix = "supplier",
  submitted = false,
}: {
  form: SupplierFormState;
  onChange: (form: SupplierFormState) => void;
  serviceTypeOptions: string[];
  idPrefix?: string;
  submitted?: boolean;
}) {
  const [touched, setTouched] = useState<Set<keyof SupplierFormState>>(new Set());
  const errors = supplierFormErrors(form);
  const visibleError = (field: keyof SupplierFormState) => (submitted || touched.has(field)) ? errors[field] : undefined;
  const fieldProps = (field: keyof SupplierFormState) => ({
    onBlur: () => setTouched((current) => new Set([...current, field])),
    "aria-invalid": Boolean(visibleError(field)),
    "aria-describedby": visibleError(field) ? `${idPrefix}-${field}-error` : undefined,
  });
  const fieldError = (field: keyof SupplierFormState) => visibleError(field) ? <p id={`${idPrefix}-${field}-error`} role="alert" className="mt-1 text-caption text-status-critical">{visibleError(field)}</p> : null;
  const update = (patch: Partial<SupplierFormState>) => onChange({ ...form, ...patch });

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div className="sm:col-span-2">
        <label htmlFor={`${idPrefix}-name`} className="mb-1 block text-label font-bold text-foreground">
          Nome *
        </label>
        <Input
          id={`${idPrefix}-name`}
          {...fieldProps("name")}
          value={form.name}
          onChange={(event) => update({ name: event.target.value })}
          placeholder="Ex.: Carlos Silva"
        />
        {fieldError("name")}
      </div>

      <div>
        <label htmlFor={`${idPrefix}-service-type`} className="mb-1 block text-label font-bold text-foreground">
          Tipo de serviço *
        </label>
        <Input
          id={`${idPrefix}-service-type`}
          {...fieldProps("serviceType")}
          value={form.serviceType}
          onChange={(event) => update({ serviceType: event.target.value })}
          placeholder="Elétrica, Hidráulica, Vistoria…"
          list={`${idPrefix}-service-types`}
        />
        {fieldError("serviceType")}
        <datalist id={`${idPrefix}-service-types`}>
          {serviceTypeOptions.map((type) => (
            <option key={type} value={type} />
          ))}
        </datalist>
        <p className="mt-1 text-caption text-muted-foreground">
          Reutilize um tipo já cadastrado para agrupar quem atende o mesmo serviço.
        </p>
      </div>

      <div>
        <label htmlFor={`${idPrefix}-kind`} className="mb-1 block text-label font-bold text-foreground">
          Vínculo
        </label>
        <select
          id={`${idPrefix}-kind`}
          value={form.kind}
          onChange={(event) => update({ kind: event.target.value as SupplierKind })}
          className="h-9 w-full rounded-lg border border-border bg-background px-2.5 text-label focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        >
          <option value="externo">Externo (terceirizado)</option>
          <option value="interno">Interno (equipe da imobiliária)</option>
        </select>
      </div>

      <div>
        <label htmlFor={`${idPrefix}-phone`} className="mb-1 block text-label font-bold text-foreground">
          Telefone
        </label>
        <PhoneInput
          className={inputClassNames}
          autoComplete="tel-national"
          id={`${idPrefix}-phone`}
          {...fieldProps("phone")}
          value={form.phone}
          onChange={(phone) => update({ phone })}
          placeholder="(11) 99999-0000"
        />
        {fieldError("phone")}
      </div>

      <div>
        <label htmlFor={`${idPrefix}-email`} className="mb-1 block text-label font-bold text-foreground">
          E-mail
        </label>
        <Input
          id={`${idPrefix}-email`}
          {...fieldProps("email")}
          type="email"
          autoComplete="email"
          value={form.email}
          onChange={(event) => update({ email: event.target.value })}
          placeholder="contato@prestador.com"
        />
        {fieldError("email")}
      </div>

      <div>
        <label htmlFor={`${idPrefix}-document`} className="mb-1 block text-label font-bold text-foreground">
          CPF/CNPJ
        </label>
        <DocumentInput
          className={inputClassNames}
          id={`${idPrefix}-document`}
          {...fieldProps("document")}
          value={form.document}
          onChange={(document) => update({ document })}
          placeholder="Opcional"
        />
        {fieldError("document")}
      </div>

      <div>
        <label htmlFor={`${idPrefix}-city`} className="mb-1 block text-label font-bold text-foreground">
          Cidade / região
        </label>
        <Input
          id={`${idPrefix}-city`}
          value={form.city}
          onChange={(event) => update({ city: event.target.value })}
          placeholder="Onde costuma atender"
        />
      </div>

      <div className="sm:col-span-2">
        <label htmlFor={`${idPrefix}-notes`} className="mb-1 block text-label font-bold text-foreground">
          Características
        </label>
        <textarea
          id={`${idPrefix}-notes`}
          rows={3}
          value={form.notes}
          onChange={(event) => update({ notes: event.target.value })}
          placeholder="Especialidades, marcas que atende, condições combinadas, disponibilidade…"
          className="w-full rounded-lg border border-border bg-background px-2.5 py-2 text-label focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        />
        <p className="mt-1 text-caption text-muted-foreground">
          O que o próximo agendamento precisa saber sem perguntar de novo.
        </p>
      </div>
    </div>
  );
}

/**
 * Seleção reutilizável de profissional/fornecedor: lista o cadastro da
 * imobiliária, permite cadastrar um novo sem sair do formulário e reaproveita
 * o registro quando o contato já existe.
 */
export function SupplierPicker({
  value,
  onChange,
  label = "Profissional / Fornecedor",
  required = false,
  onSupplierSelected,
  inspectorsOnly = false,
}: {
  value: string;
  onChange: (supplierId: string) => void;
  label?: string;
  required?: boolean;
  inspectorsOnly?: boolean;
  onSupplierSelected?: (supplier: Supplier | undefined) => void;
}) {
  const queryClient = useQueryClient();
  const selectId = useId();
  const [form, setForm] = useState<SupplierFormState>(EMPTY_FORM);
  const [editing, setEditing] = useState<Supplier | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [submitted, setSubmitted] = useState(false);

  const suppliersQuery = useQuery({
    queryKey: [SUPPLIER_QUERY_KEY, "list"],
    queryFn: async () => {
      return collectPages<Supplier>((paging) => supplierService.getAll(paging));
    },
    staleTime: 30_000,
  });

  const serviceTypesQuery = useQuery({
    queryKey: [SUPPLIER_QUERY_KEY, "service-types"],
    queryFn: async () => (await supplierService.getServiceTypes())?.data ?? [],
    staleTime: 60_000,
  });

  const suppliers = suppliersQuery.data ?? [];
  const activeVendors = useMemo(() => suppliers.filter((supplier) => supplier.active && (!inspectorsOnly || isInspectorService(supplier.service_type))), [suppliers, inspectorsOnly]);
  const options = useMemo<SearchableSelectOption[]>(
    () =>
      activeVendors.map((supplier) => ({
        value: supplier.id,
        label: supplierDisplayName(supplier),
        searchText: [supplier.name, supplier.service_type, supplier.phone, supplier.email, supplier.notes]
          .filter(Boolean)
          .join(" "),
      })),
    [activeVendors],
  );

  const serviceTypeOptions = useMemo(
    () =>
      Array.from(
        new Set([
          ...(serviceTypesQuery.data ?? []),
          ...suppliers.map((supplier) => supplier.service_type).filter(Boolean),
        ]),
      ).sort(),
    [serviceTypesQuery.data, suppliers],
  );

  const selectedSupplier = useMemo(
    () => suppliers.find((supplier) => supplier.id === value),
    [value, suppliers],
  );

  useEffect(() => {
    onSupplierSelected?.(selectedSupplier);
  }, [onSupplierSelected, selectedSupplier]);

  const saveMutation = useMutation({
    mutationFn: async (payload: CreateSupplierDTO) =>
      editing ? supplierService.update(editing.id, payload) : supplierService.create(payload),
    onSuccess: (response) => {
      const saved = response?.data as Supplier | undefined;
      queryClient.invalidateQueries({ queryKey: [SUPPLIER_QUERY_KEY] });
      if (inspectorsOnly && saved && (!saved.active || !isInspectorService(saved.service_type))) {
        toast.error("Este contato já pertence a um profissional inativo ou de outro serviço. Atualize o cadastro antes de selecioná-lo como vistoriador.");
        return;
      }
      if (saved?.id && (!inspectorsOnly || (saved.active && isInspectorService(saved.service_type)))) {
        queryClient.setQueryData<Supplier[]>([SUPPLIER_QUERY_KEY, "list"], (current = []) => [saved, ...current.filter((item) => item.id !== saved.id)]);
        onChange(saved.id);
        onSupplierSelected?.(saved);
      }
      toast.success(editing ? "Profissional atualizado." : "Profissional cadastrado.");
      setFormOpen(false);
      setEditing(null);
      setForm(EMPTY_FORM);
    },
    onError: () => toast.error("Não foi possível salvar o profissional."),
  });

  const deactivateMutation = useMutation({
    mutationFn: async (supplier: Supplier) => supplierService.deactivate(supplier.id),
    onSuccess: (_response, supplier) => {
      queryClient.invalidateQueries({ queryKey: [SUPPLIER_QUERY_KEY] });
      if (supplier.id === value) onChange("");
      toast.success("Profissional desativado e fora da seleção.");
    },
    onError: () => toast.error("Não foi possível desativar o profissional."),
  });

  const openCreate = useCallback(() => {
    setSubmitted(false);
    setEditing(null);
    setForm({ ...EMPTY_FORM, serviceType: inspectorsOnly ? "Vistoria" : "" });
    setFormOpen(true);
  }, [inspectorsOnly]);

  const openEdit = useCallback((supplier: Supplier) => {
    setSubmitted(false);
    setEditing(supplier);
    setForm(formFromSupplier(supplier));
    setFormOpen(true);
  }, []);

  const submitForm = () => {
    if (saveMutation.isPending) return;
    setSubmitted(true);
    const invalid = validateSupplierForm(form);
    if (invalid) {
      toast.error(invalid);
      const field = Object.keys(supplierFormErrors(form))[0];
      const suffix = field === "serviceType" ? "service-type" : field;
      document.getElementById(`${selectId}-${suffix}`)?.focus();
      return;
    }
    if (inspectorsOnly && !isInspectorService(form.serviceType)) {
      toast.error("Escolha o tipo Vistoria ou Vistoriador para este agendamento.");
      return;
    }
    saveMutation.mutate(toDTO(form));
  };

  return (
    <div className="space-y-2">
      <div className="flex items-end justify-between gap-2">
        <label htmlFor={selectId} className="block text-label font-bold text-foreground">
          {label}
          {required ? " *" : ""}
        </label>
        <Button type="button" variant="ghost" size="sm" onClick={openCreate} className="h-7 gap-1 px-2 text-caption">
          <Plus className="h-3.5 w-3.5" /> Novo
        </Button>
      </div>

      {suppliersQuery.isLoading ? (
        <div className="flex h-9 items-center gap-2 text-caption text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" /> Carregando profissionais…
        </div>
      ) : (
        <SearchableSelect
          id={selectId}
          value={value}
          onValueChange={onChange}
          options={options}
          placeholder="Selecione um profissional cadastrado…"
          searchPlaceholder="Buscar por nome, serviço, telefone…"
          emptyMessage="Nenhum profissional cadastrado. Use “Novo” para cadastrar."
          aria-label={label}
        />
      )}

      {suppliersQuery.isError && (
        <p className="text-caption font-bold text-status-warning">
          Não foi possível carregar o cadastro de profissionais.
        </p>
      )}

      {selectedSupplier ? (
        <div className="rounded-lg border border-border bg-muted/30 p-3">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="flex items-center gap-1.5 text-label font-bold text-foreground">
                <Briefcase className="h-3.5 w-3.5 text-primary-600" />
                {selectedSupplier.service_type}
                <span className="text-caption font-medium text-muted-foreground">
                  {selectedSupplier.kind === "interno" ? "equipe interna" : "externo"}
                </span>
              </p>
              {supplierContact(selectedSupplier) ? (
                <p className="mt-1 flex items-center gap-1.5 text-caption text-muted-foreground">
                  <Phone className="h-3 w-3" /> {supplierContact(selectedSupplier)}
                </p>
              ) : (
                <p className="mt-1 text-caption font-bold text-status-warning">
                  Cadastro sem contato: complete para conseguir acionar.
                </p>
              )}
              {selectedSupplier.notes && (
                <p className="mt-1 text-caption text-muted-foreground">{selectedSupplier.notes}</p>
              )}
              {selectedSupplier.city && (
                <p className="mt-1 text-caption text-muted-foreground">Atende: {selectedSupplier.city}</p>
              )}
            </div>
            <div className="flex shrink-0 gap-1">
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-8"
                title="Editar cadastro"
                onClick={() => openEdit(selectedSupplier)}
              >
                <Pencil className="h-3.5 w-3.5" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-8 text-muted-foreground hover:text-status-critical"
                title="Desativar (sai da seleção, histórico mantido)"
                onClick={() => deactivateMutation.mutate(selectedSupplier)}
                loading={deactivateMutation.isPending}
              >
                <UserRoundCheck className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        </div>
      ) : (
        <p className="text-caption text-muted-foreground">
          Escolher um profissional cadastrado evita redigitar contato e características a cada chamado.
        </p>
      )}

      {/* Painel lateral (padrão do projeto para formulários). Abre por cima do
          painel de agendamento: o overlay escurece o que está atrás — inclusive
          o próprio agendamento — e o foco volta ao botão "Novo" ao fechar. */}
      <Sheet
        open={formOpen}
        onOpenChange={(open: boolean) => {
          setFormOpen(open);
          if (!open) {
            setEditing(null);
            setForm(EMPTY_FORM);
          }
        }}
      >
        <SheetContent
          side="right"
          expandable
          className="z-[60] w-full gap-0 overflow-hidden p-0 sm:max-w-lg"
          data-testid="supplier-form-panel"
        >
          <div className="flex items-start justify-between gap-3 border-b border-border px-6 py-4 pr-20">
            <div className="min-w-0">
              <SheetTitle className="flex items-center gap-2 text-xl font-bold text-foreground">
                <Briefcase className="h-5 w-5 text-primary-600" />
                {editing ? "Editar profissional" : "Novo profissional"}
              </SheetTitle>
              <SheetDescription className="mt-1 text-xs text-muted-foreground">
                Contato, tipo de serviço e características ficam no cadastro para reutilizar no próximo chamado.
              </SheetDescription>
            </div>
          </div>

          <div className="flex-1 overflow-y-auto px-6 py-5">
            <SupplierFields form={form} onChange={setForm} serviceTypeOptions={serviceTypeOptions} idPrefix={selectId} submitted={submitted} />
          </div>

          <div className="flex justify-end gap-2 border-t border-border bg-muted px-6 py-4">
            <Button type="button" variant="outline" onClick={() => setFormOpen(false)}>
              Cancelar
            </Button>
            <Button type="button" onClick={submitForm} loading={saveMutation.isPending}>
              {editing ? "Salvar alterações" : "Cadastrar profissional"}
            </Button>
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}