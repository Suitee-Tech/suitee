import * as React from "react";
import type { Control, FieldPath, FieldValues } from "react-hook-form";

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "./form";
import { cn } from "./utils";
import { Input, inputClassNames } from "./input";
import { Textarea } from "./textarea";
import { Checkbox } from "./checkbox";
import { MoneyInput } from "./money-input";
import { PhoneInput } from "./phone-input";
import { CepInput } from "./cep-input";
import { DocumentInput, type DocumentMode } from "./document-input";
import { PercentInput } from "./percent-input";
import { SearchableSelect } from "./searchable-select";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./select";

// Wrappers de campo padronizados: cada um compõe FormField (Controller) +
// FormItem/FormLabel/FormControl/FormMessage com o input certo, já com
// label, asterisco de obrigatório (visual; a regra real é do zod), erro e
// acessibilidade (aria-invalid/aria-describedby via FormControl). Genéricos
// sobre os valores do form, então `name` é checado em compilação.

type BaseFieldProps<T extends FieldValues> = {
  control: Control<T>;
  name: FieldPath<T>;
  label?: React.ReactNode;
  description?: React.ReactNode;
  required?: boolean;
  /** classes do wrapper (ex.: col-span no grid). */
  className?: string;
};

function Frame({
  label,
  required,
  description,
  className,
  children,
}: {
  label?: React.ReactNode;
  required?: boolean;
  description?: React.ReactNode;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <FormItem className={className}>
      {label != null && label !== "" && (
        <FormLabel required={required}>{label}</FormLabel>
      )}
      <FormControl aria-required={required || undefined}>{children}</FormControl>
      {description != null && description !== "" && <FormDescription>{description}</FormDescription>}
      <FormMessage />
    </FormItem>
  );
}

type InputExtras = {
  placeholder?: string;
  disabled?: boolean;
  inputClassName?: string;
};

type NativeInputProps = Omit<
  React.ComponentProps<"input">,
  "value" | "onChange" | "onBlur" | "name" | "ref" | "type" | "className"
>;

export function TextField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  required,
  className,
  type = "text",
  placeholder,
  disabled,
  inputClassName,
  autoComplete,
  inputProps,
}: BaseFieldProps<T> &
  InputExtras & {
    type?: React.HTMLInputTypeAttribute;
    autoComplete?: string;
    /** Atributos nativos extras (inputMode, maxLength, autoFocus, pattern…). */
    inputProps?: NativeInputProps;
  }) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <Frame label={label} required={required} description={description} className={className}>
          <Input
            {...inputProps}
            type={type}
            placeholder={placeholder}
            disabled={disabled ?? field.disabled}
            autoComplete={autoComplete}
            className={inputClassName}
            name={field.name}
            ref={field.ref}
            value={(field.value as string | undefined) ?? ""}
            onChange={field.onChange}
            onBlur={field.onBlur}
          />
        </Frame>
      )}
    />
  );
}

export function TextareaField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  required,
  className,
  placeholder,
  disabled,
  inputClassName,
  rows,
}: BaseFieldProps<T> & InputExtras & { rows?: number }) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <Frame label={label} required={required} description={description} className={className}>
          <Textarea
            placeholder={placeholder}
            disabled={disabled ?? field.disabled}
            rows={rows}
            className={inputClassName}
            name={field.name}
            ref={field.ref}
            value={(field.value as string | undefined) ?? ""}
            onChange={field.onChange}
            onBlur={field.onBlur}
          />
        </Frame>
      )}
    />
  );
}

export function DateField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  required,
  className,
  disabled,
  inputClassName,
  min,
  max,
}: BaseFieldProps<T> & InputExtras & { min?: string; max?: string }) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <Frame label={label} required={required} description={description} className={className}>
          <Input
            type="date"
            disabled={disabled ?? field.disabled}
            min={min}
            max={max}
            className={inputClassName}
            name={field.name}
            ref={field.ref}
            value={(field.value as string | undefined) ?? ""}
            onChange={field.onChange}
            onBlur={field.onBlur}
          />
        </Frame>
      )}
    />
  );
}

export function PhoneField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  required,
  className,
  placeholder = "(00) 00000-0000",
  disabled,
  inputClassName,
}: BaseFieldProps<T> & InputExtras) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <Frame label={label} required={required} description={description} className={className}>
          <PhoneInput
            placeholder={placeholder}
            disabled={disabled ?? field.disabled}
            className={cn(inputClassNames, inputClassName)}
            name={field.name}
            ref={field.ref}
            value={field.value as string | undefined}
            onChange={field.onChange}
            onBlur={field.onBlur}
          />
        </Frame>
      )}
    />
  );
}

export function CepField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  required,
  className,
  placeholder = "00000-000",
  disabled,
  inputClassName,
  onCep,
}: BaseFieldProps<T> & InputExtras & { onCep?: (cep: string) => void }) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <Frame label={label} required={required} description={description} className={className}>
          <CepInput
            placeholder={placeholder}
            disabled={disabled ?? field.disabled}
            className={cn(inputClassNames, inputClassName)}
            name={field.name}
            ref={field.ref}
            value={field.value as string | undefined}
            onChange={field.onChange}
            onBlur={() => {
              field.onBlur();
              const cep = String(field.value || "");
              if (onCep && cep.length === 8) onCep(cep);
            }}
          />
        </Frame>
      )}
    />
  );
}

export function DocumentField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  required,
  className,
  mode = "auto",
  placeholder,
  disabled,
  inputClassName,
}: BaseFieldProps<T> & InputExtras & { mode?: DocumentMode }) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <Frame label={label} required={required} description={description} className={className}>
          <DocumentInput
            mode={mode}
            placeholder={placeholder}
            disabled={disabled ?? field.disabled}
            className={cn(inputClassNames, inputClassName)}
            name={field.name}
            ref={field.ref}
            value={field.value as string | undefined}
            onChange={field.onChange}
            onBlur={field.onBlur}
          />
        </Frame>
      )}
    />
  );
}

export function MoneyField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  required,
  className,
  placeholder = "R$ 0,00",
  disabled,
  inputClassName,
  fractionDigits,
  withSymbol = true,
}: BaseFieldProps<T> & InputExtras & { fractionDigits?: 0 | 2; withSymbol?: boolean }) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <Frame label={label} required={required} description={description} className={className}>
          <MoneyInput
            placeholder={placeholder}
            disabled={disabled ?? field.disabled}
            fractionDigits={fractionDigits}
            withSymbol={withSymbol}
            className={cn(inputClassNames, inputClassName)}
            name={field.name}
            ref={field.ref}
            value={field.value as number | undefined}
            onChange={field.onChange}
            onBlur={field.onBlur}
          />
        </Frame>
      )}
    />
  );
}

export function PercentField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  required,
  className,
  placeholder = "0",
  disabled,
  inputClassName,
  fractionDigits,
}: BaseFieldProps<T> & InputExtras & { fractionDigits?: number }) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <Frame label={label} required={required} description={description} className={className}>
          <PercentInput
            placeholder={placeholder}
            disabled={disabled ?? field.disabled}
            fractionDigits={fractionDigits}
            className={cn(inputClassNames, inputClassName)}
            name={field.name}
            ref={field.ref}
            value={field.value as number | undefined}
            onChange={field.onChange}
            onBlur={field.onBlur}
          />
        </Frame>
      )}
    />
  );
}

export type SelectOption = { value: string; label: React.ReactNode; searchText?: string };

export function SelectField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  required,
  className,
  options,
  placeholder = "Selecione…",
  disabled,
  triggerClassName,
  searchable = false,
  searchPlaceholder = "Buscar…",
  emptyMessage = "Nenhuma opção encontrada.",
}: BaseFieldProps<T> & {
  options: SelectOption[];
  placeholder?: string;
  disabled?: boolean;
  triggerClassName?: string;
  searchable?: boolean;
  searchPlaceholder?: string;
  emptyMessage?: string;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem className={className}>
          {label != null && label !== "" && (
            <FormLabel>
              {label}
              {required ? <span className="text-destructive"> *</span> : null}
            </FormLabel>
          )}
          {searchable ? (
            <FormControl>
              <SearchableSelect
                value={(field.value as string | undefined) ?? ""}
                onValueChange={field.onChange}
                options={options}
                placeholder={placeholder}
                searchPlaceholder={searchPlaceholder}
                emptyMessage={emptyMessage}
                disabled={disabled ?? field.disabled}
                name={field.name}
                ref={field.ref}
                onBlur={field.onBlur}
                className={triggerClassName}
              />
            </FormControl>
          ) : (
            <Select
              value={(field.value as string | undefined) || undefined}
              onValueChange={field.onChange}
              disabled={disabled ?? field.disabled}
            >
              <FormControl>
                <SelectTrigger
                  className={triggerClassName}
                  name={field.name}
                  ref={field.ref}
                  onBlur={field.onBlur}
                >
                  <SelectValue placeholder={placeholder} />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                {options.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          {description != null && description !== "" && (
            <FormDescription>{description}</FormDescription>
          )}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

export function CheckboxField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  className,
  disabled,
}: BaseFieldProps<T> & { disabled?: boolean }) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem className={`flex flex-row items-start gap-2 space-y-0 ${className ?? ""}`}>
          <FormControl>
            <Checkbox
              ref={field.ref}
              checked={!!field.value}
              onCheckedChange={field.onChange}
              disabled={disabled ?? field.disabled}
            />
          </FormControl>
          <div className="grid gap-1 leading-none">
            {label != null && label !== "" && (
              <FormLabel className="font-normal">{label}</FormLabel>
            )}
            {description != null && description !== "" && (
              <FormDescription>{description}</FormDescription>
            )}
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );
}
