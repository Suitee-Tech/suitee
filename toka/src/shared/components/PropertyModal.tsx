import { Controller, useForm, useFieldArray } from "react-hook-form";
import {
  Building2,
  MapPin,
  DollarSign,
  Home,
  Bed,
  Bath,
  Square,
  User,
  Briefcase,
  Plus,
  Trash2,
  ChevronsUpDown,
  Check,
  Loader2,
} from "lucide-react";
import { clsx } from "clsx";
import { useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { AMENITY_SUGGESTIONS, Property, PropertyStatus } from "../data";
import { useAgentsContext } from "../context/AgentsContext";
import { useAuth } from "../context/AuthContext";
import { useLeadsContext } from "../context/LeadsContext";
import { usePropertiesContext } from "../context/PropertiesContext";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "../ui/command";
import { TagInput } from "../ui/tag-input";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { Avatar, AvatarFallback, AvatarImage } from "../ui/avatar";
import { Switch } from "../ui/switch";
import { PropertyMediaSection } from "./PropertyMediaSection";
import { MapPinPicker } from "./MapPinPicker";
import { propertyService, type PropertyDTO } from "../services/property.service";
import { stateCodeFromName, stateNameFromCode } from "../utils/uf";
import { lookupCep } from "../services/cep.service";
import { mapPropertyDtoToProperty } from "../lib/properties/mappers";
import {
  propertiesListQueryKey,
  propertyFallbackQueryKey,
  propertyQueryKey,
} from "../properties/queries";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";
import { MoneyInput } from "../ui/money-input";
import { formatCep, parseCep } from "../utils/masks";
import { Button } from "../ui/button";
import { notifyApiError } from "../services/errorToast";

interface PropertyModalProps {
  isOpen: boolean;
  onClose: () => void;
  property?: Property;
  /** Chamado com o imóvel recém-criado — quem abriu o cadastro de dentro de outro
   *  fluxo (ex.: seletor de imóvel do novo negócio) seleciona o resultado. */
  onCreated?: (property: Property) => void;
}

type PropertyFormData = Omit<Property, "id">;
type PropertyImage = NonNullable<Property["images"]>[number];
type UploadedPropertyImage = Pick<
  Awaited<ReturnType<typeof propertyService.uploadImage>>,
  "url" | "filename"
>;

export function resolveCreatedPropertyImages(
  images: PropertyImage[],
  uploadedByPendingId: ReadonlyMap<string, UploadedPropertyImage>,
): PropertyImage[] {
  return images
    .flatMap((img): PropertyImage[] => {
      const pendingId =
        typeof img.filename === "string" && img.filename.startsWith("pending:")
          ? img.filename
          : null;

      if (pendingId) {
        const uploaded = uploadedByPendingId.get(pendingId);
        return uploaded?.url
          ? [{ ...img, url: uploaded.url, filename: uploaded.filename }]
          : [];
      }

      return String(img.url || "").startsWith("blob:") ? [] : [img];
    })
    .map((img, order) => ({ ...img, order }));
}

const PROPERTY_STATUSES: PropertyStatus[] = [
  "Em Captação",
  "Em Portfólio",
  "Anunciado",
  "Em Atendimento",
  "Em Negociação",
  "Em Proposta",
  "Em Análise",
  "Em Contrato",
  "Alugado",
  "Vendido",
  "Em Administração",
  "Em Rescisão",
  "Disponível",
  "Inativo",
];

const PROPERTY_TYPES = [
  "Apartamento",
  "Casa",
  "Cobertura",
  "Terreno",
  "Comercial",
  "Sítio/Fazenda",
  "Studio",
  "Loft",
];

export function PropertyModal({ isOpen, onClose, property, onCreated }: PropertyModalProps) {
  const queryClient = useQueryClient();
  const { addProperty, updateProperty, setProperties } = usePropertiesContext();
  const { leads } = useLeadsContext();
  const { agents } = useAgentsContext();
  const { user } = useAuth();
  const tenantId = user?.current_company_id;
  const beginScopedRequest = useRequestScopeGuard(tenantId ?? "");
  const pendingFilesRef = useRef<Map<string, File>>(new Map());
  const pendingDeletesRef = useRef<string[]>([]);

  const displayAgents = useMemo(() => {
    const currentAgents = Array.isArray(agents) ? [...agents] : [];
    if (user && !currentAgents.some((a) => a.id === user.id || (a as any).email === user.email)) {
      currentAgents.push({
        id: user.id,
        name:
          user.fullname ||
          `${user.firstname || ""} ${user.lastname || ""}`.trim() ||
          "Você",
        role: user.role || user.permission || "Corretor",
        avatar: user.avatar_url || "",
      });
    }
    return currentAgents;
  }, [agents, user]);

  const {
    register,
    control,
    handleSubmit,
    formState: { errors, isSubmitting },
    reset,
    watch,
    setValue,
    getValues,
    trigger,
  } = useForm<PropertyFormData>({
    defaultValues: property
      ? {
        title: property.title,
        address: property.address,
        price: property.price,
        type: property.type,
        status: property.status,
        beds: property.beds,
        baths: property.baths,
        area: property.area,
        image: property.image,
        images:
          property.images && property.images.length > 0
            ? property.images
            : property.image
              ? [{ url: property.image, order: 0 }]
              : [],
        videos: property.videos || [],
        description: property.description || "",
        condominiumFee: property.condominiumFee || 0,
        propertyTax: property.propertyTax || 0,
        suites: property.suites || 0,
        parkingSpaces: property.parkingSpaces || 0,
        amenities: property.amenities || [],
        owners: property.owners || [],
        capturingBrokers: property.capturingBrokers || [],
        availableFor: property.availableFor || ["sale"],
        intermediationContractId: property.intermediationContractId || "",
        captureExclusivity: property.captureExclusivity || "open",
        partnershipPortfolio: property.partnershipPortfolio ?? false,
      }
      : {
        title: "",
        address: {
          street: "",
          number: "",
          complement: "",
          neighborhood: "",
          city: "",
          state: "",
          stateCode: "",
          postalCode: "",
          country: "Brasil",
          countryCode: "BR",
        },
        price: 0,
        type: "Apartamento",
        status: "Em Captação",
        beds: 0,
        baths: 0,
        suites: 0,
        parkingSpaces: 0,
        area: 0,
        image:
          "",
        images: [],
        videos: [],
        description: "",
        condominiumFee: 0,
        propertyTax: 0,
        amenities: [],
        owners: [],
        capturingBrokers: user ? [{ id: user.id, role: "captador" }] : [],
        availableFor: ["sale"],
        intermediationContractId: "",
        captureExclusivity: "open",
        partnershipPortfolio: false,
      },
  });

  const {
    fields: ownerFields,
    append: appendOwner,
    remove: removeOwner,
  } = useFieldArray({
    control,
    name: "owners",
  });

  const {
    fields: brokerFields,
    append: appendBroker,
    remove: removeBroker,
  } = useFieldArray({
    control,
    name: "capturingBrokers",
  });

  const [openOwnerCombobox, setOpenOwnerCombobox] = useState<Record<string, boolean>>({});
  const [openBrokerCombobox, setOpenBrokerCombobox] = useState<Record<string, boolean>>({});
  // Enquanto alguma imagem está sendo carregada/validada (ou falhou), a
  // criação fica bloqueada — garante que nenhum imóvel nasça sem as imagens.
  const [imagesLoadingCount, setImagesLoadingCount] = useState(0);
  const [imagesHaveErrors, setImagesHaveErrors] = useState(false);

  const postalCode = watch("address.postalCode");
  const addressForGeocode = watch("address");
  const latitude = watch("address.latitude");
  const longitude = watch("address.longitude");
  const captureExclusivity = watch("captureExclusivity");
  const partnershipPortfolio = watch("partnershipPortfolio");
  const exclusivityLocksSharing = captureExclusivity === "exclusive";
  const postalCodeField = register("address.postalCode", {
    required: "Informe o CEP",
    validate: (v) => {
      const digits = String(v || "").replace(/\D/g, "");
      if (digits.length === 8) return true;
      if (digits.length === 0) return "CEP inválido — informe um CEP válido (ex.: 01310-100)";
      return `CEP incompleto (${digits.length}/8 dígitos) — ex.: 01310-100`;
    },
  });
  const [cepLoading, setCepLoading] = useState(false);
  const handleCepLookup = async () => {
    const cep = String(watch("address.postalCode") || "").replace(/\D/g, "");
    if (cep.length !== 8) return;
    setCepLoading(true);
    let filled = false;
    try {
      const result = await lookupCep(cep);
      if (result) {
        // Preenche sem validar para não "acender" erro enquanto carrega.
        if (result.street) setValue("address.street", result.street, { shouldDirty: true, shouldValidate: false });
        if (result.neighborhood) setValue("address.neighborhood", result.neighborhood, { shouldDirty: true, shouldValidate: false });
        if (result.city) setValue("address.city", result.city, { shouldDirty: true, shouldValidate: false });
        if (result.state) setValue("address.state", result.state, { shouldDirty: true, shouldValidate: false });
        if (result.stateCode) setValue("address.stateCode", result.stateCode, { shouldDirty: true, shouldValidate: false });
        filled = true;
      }
    } finally {
      setCepLoading(false);
      // Só depois da consulta valida os campos preenchidos (evita erro durante o load).
      if (filled) {
        trigger(["address.street", "address.neighborhood", "address.city", "address.state", "address.stateCode"]).catch(() => {});
      }
    }
  };

  if (!isOpen) return null;

  const onSubmit = async (data: PropertyFormData) => {
    const isCurrent = beginScopedRequest();
    try {
      // Bloqueio de persistência: imagem ainda carregando ou com falha não
      // pode virar imóvel/imagem quebrada.
      if (imagesLoadingCount > 0) {
        toast.info("Aguarde as imagens terminarem de carregar para salvar.");
        return;
      }
      if (imagesHaveErrors) {
        toast.error("Há imagem(ns) com falha de carregamento — remova-as ou tente novamente antes de salvar.");
        return;
      }
      // O backend exige `state` (nome) E `stateCode` (sigla). O formulário
      // costuma ter só a sigla digitada — completa o nome (e vice-versa)
      // antes de enviar, para nunca mandar `state: ""`.
      const stateName = String(data.address.state || "").trim();
      const stateCode = String(data.address.stateCode || "").trim().toUpperCase();
      const resolvedState = stateName || (stateCode ? stateNameFromCode(stateCode) : "");
      const resolvedCode = stateCode || stateCodeFromName(stateName);
      data.address = {
        ...data.address,
        state: resolvedState || resolvedCode,
        stateCode: resolvedCode,
      };

      const normalizedImages = (data.images || [])
        .filter((img) => img?.url && String(img.url).trim() !== "")
        .map((img, idx) => ({ ...img, order: idx }));

      const normalizedVideos = (data.videos || [])
        .filter((v) => v?.url)
        .map((v, idx) => ({ ...v, order: idx }));

      const image = normalizedImages[0]?.url || data.image;

      if (property) {
        const uploadedImages = [];
        for (const img of normalizedImages) {
          if (
            img.filename &&
            typeof img.filename === "string" &&
            img.filename.startsWith("pending:")
          ) {
            const f = pendingFilesRef.current.get(img.filename);
            if (f) {
              const res = await propertyService.uploadImage(property.id, f);
              if (!isCurrent()) return;
              pendingFilesRef.current.delete(img.filename);
              uploadedImages.push({ ...img, url: res.url, filename: res.filename });
            } else {
              uploadedImages.push(img);
            }
          } else {
            uploadedImages.push(img);
          }
        }

        const updated = await updateProperty(property.id, {
          ...data,
          image,
          images: uploadedImages,
          videos: normalizedVideos,
        });
        if (!updated || !isCurrent()) return;

        const deletes = pendingDeletesRef.current;
        pendingDeletesRef.current = [];
        for (const filename of deletes) {
          try {
            await propertyService.deleteImage(property.id, filename);
            if (!isCurrent()) return;
          } catch (e) {
            console.error(e);
          }
        }
      } else {
        const externalImages = normalizedImages.filter(
          (img) => !String(img.url || "").startsWith("blob:"),
        );
        const created = await addProperty({
          ...data,
          image: externalImages[0]?.url || image,
          images: externalImages,
          videos: normalizedVideos,
        });
        if (!created || !isCurrent()) return;
        if (created.id) {
          const pendingUploads: {
            pendingId: string;
            promise: ReturnType<typeof propertyService.uploadImage>;
          }[] = [];
          for (const img of normalizedImages) {
            if (
              img.filename &&
              typeof img.filename === "string" &&
              img.filename.startsWith("pending:")
            ) {
              const f = pendingFilesRef.current.get(img.filename);
              if (f) {
                pendingUploads.push({
                  pendingId: img.filename,
                  promise: propertyService.uploadImage(created.id, f),
                });
              }
            }
          }

          let syncedProperty = created;
          let syncedPropertyDto: PropertyDTO | null = null;

          if (pendingUploads.length > 0) {
            const results = await Promise.allSettled(
              pendingUploads.map(({ promise }) => promise),
            );
            if (!isCurrent()) return;
            const uploadedByPendingId = new Map<string, UploadedPropertyImage>();
            let failed = 0;

            for (let i = 0; i < results.length; i++) {
              const result = results[i];
              const pendingId = pendingUploads[i].pendingId;
              pendingFilesRef.current.delete(pendingId);

              if (result.status === "rejected") {
                failed += 1;
                continue;
              }
              if (result.value.persist_error) {
                failed += 1;
                continue;
              }

              uploadedByPendingId.set(pendingId, {
                url: result.value.url,
                filename: result.value.filename,
              });
              if (result.value.property) {
                syncedPropertyDto = result.value.property;
              }
            }

            const resolvedImages = resolveCreatedPropertyImages(
              normalizedImages,
              uploadedByPendingId,
            );

            const lastDto = syncedPropertyDto;
            if (lastDto) {
              const mergedDto: PropertyDTO = {
                ...lastDto,
                media: {
                  ...lastDto.media,
                  images: resolvedImages,
                },
              };
              syncedPropertyDto = mergedDto;
              syncedProperty = mapPropertyDtoToProperty(mergedDto);
            } else {
              syncedProperty = {
                ...created,
                image: resolvedImages[0]?.url || created.image,
                images: resolvedImages,
              };
            }

            if (failed > 0 && isCurrent()) {
              toast.error(
                `${failed} imagem(ns) não puderam ser salvas no imóvel. Repita a tentativa para garantir que as fotos fiquem persistidas.`,
              );
              // Mantém o formulário aberto com as pendentes — reenviar fará o
              // re-upload; fechar aqui deixaria o imóvel sem as fotos.
              return;
            }
          }

          if (!isCurrent()) return;
          setProperties((current) =>
            current.map((item) => (item.id === created.id ? syncedProperty : item)),
          );
          queryClient.setQueryData(propertyFallbackQueryKey(created.id, tenantId), syncedProperty);

          if (syncedPropertyDto) {
            queryClient.setQueryData(propertyQueryKey(created.id, tenantId), syncedPropertyDto);
            for (const queryKey of [
              propertiesListQueryKey({}, tenantId),
              propertiesListQueryKey({ limit: 1000, offset: 0 }, tenantId),
            ]) {
              queryClient.setQueryData<PropertyDTO[]>(queryKey, (current) => {
                if (!Array.isArray(current)) return current;
                const exists = current.some((item) => item.id === created.id);
                return exists
                  ? current.map((item) =>
                    item.id === created.id ? syncedPropertyDto! : item,
                  )
                  : [syncedPropertyDto!, ...current];
              });
            }
          }

          await queryClient.invalidateQueries({ queryKey: ["properties"] });
          if (isCurrent()) onCreated?.(created);
          if (!isCurrent()) return;
        }
      }

      if (!isCurrent()) return;
      reset();
      onClose();
    } catch (error) {
      if (!isCurrent()) return;
      notifyApiError(error, {
        context: property ? "Salvar imóvel" : "Criar imóvel",
      });
    }
  };

  return (
    <div className="flex flex-col h-full font-sans bg-card overflow-hidden">
      <div className="flex-1 overflow-y-auto p-6">
        <form id="property-form" onSubmit={handleSubmit(onSubmit)} className="space-y-8 flex flex-col">
          <div className="space-y-4">
            <h3 className="text-sm font-bold text-foreground border-b border-border pb-2 mb-4 flex items-center gap-2">
              <Home className="w-4 h-4 text-primary-500 dark:text-primary-400" /> Informações Básicas
            </h3>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div className="md:col-span-2">
                <label className="flex items-center gap-2 text-sm font-bold text-foreground mb-2">
                  Título
                </label>
                <input
                  {...register("title", { required: "Título é obrigatório" })}
                  className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                  placeholder="Ex: Cobertura Duplex no Leblon"
                />
                {errors.title && (
                  <span className="text-xs text-status-critical mt-1">
                    {errors.title.message}
                  </span>
                )}
              </div>

              <div className="md:col-span-2">
                <label className="flex items-center gap-2 text-sm font-bold text-foreground mb-2">
                  <MapPin className="w-4 h-4 text-muted-foreground" /> Endereço
                </label>
                <div className="grid grid-cols-1 sm:grid-cols-6 gap-3">
                  <div className="sm:col-span-2">
                    <label className="block text-xs font-bold text-foreground mb-1.5">CEP</label>
                    <input
                      {...postalCodeField}
                      value={formatCep(postalCode || "")}
                      inputMode="numeric"
                      disabled={cepLoading}
                      onBlur={handleCepLookup}
                      onChange={(e) =>
                        setValue("address.postalCode", parseCep(e.target.value), {
                          shouldDirty: true,
                          shouldValidate: true,
                        })
                      }
                      className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                      placeholder="00000-000"
                    />
                    {errors.address?.postalCode && (
                      <span className="text-xs text-status-critical mt-1">{errors.address.postalCode.message}</span>
                    )}
                  </div>
                  <div className="sm:col-span-4">
                    <label className="block text-xs font-bold text-foreground mb-1.5">Rua / Avenida</label>
                    <input
                      {...register("address.street", { required: "Rua é obrigatória" })}
                      className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                      placeholder="Ex: Rua das Flores"
                    />
                    {errors.address?.street && (
                      <span className="text-xs text-status-critical mt-1">{errors.address.street.message}</span>
                    )}
                  </div>
                  <div className="sm:col-span-2">
                    <label className="block text-xs font-bold text-foreground mb-1.5">Número</label>
                    <input
                      {...register("address.number", { required: "Número é obrigatório" })}
                      className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                      placeholder="Ex: 100"
                    />
                    {errors.address?.number && (
                      <span className="text-xs text-status-critical mt-1">{errors.address.number.message}</span>
                    )}
                  </div>
                  <div className="sm:col-span-4">
                    <label className="block text-xs font-bold text-foreground mb-1.5">Complemento</label>
                    <input
                      {...register("address.complement")}
                      className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                      placeholder="Apto, bloco…"
                    />
                  </div>
                  <div className="sm:col-span-3">
                    <label className="block text-xs font-bold text-foreground mb-1.5">Bairro</label>
                    <input
                      {...register("address.neighborhood", { required: "Bairro é obrigatório" })}
                      className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                      placeholder="Ex: Centro"
                    />
                    {errors.address?.neighborhood && (
                      <span className="text-xs text-status-critical mt-1">{errors.address.neighborhood.message}</span>
                    )}
                  </div>
                  <div className="sm:col-span-3">
                    <label className="block text-xs font-bold text-foreground mb-1.5">Cidade</label>
                    <input
                      {...register("address.city", { required: "Cidade é obrigatória" })}
                      className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                      placeholder="Ex: São Paulo"
                    />
                    {errors.address?.city && (
                      <span className="text-xs text-status-critical mt-1">{errors.address.city.message}</span>
                    )}
                  </div>
                  <div className="col-span-1">
                    <label className="block text-xs font-bold text-foreground mb-1.5">UF</label>
                    <input
                      {...register("address.stateCode", { required: "UF é obrigatória" })}
                      className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium uppercase"
                      placeholder="SP"
                      maxLength={2}
                    />
                    {errors.address?.stateCode && (
                      <span className="text-xs text-status-critical mt-1">{errors.address.stateCode.message}</span>
                    )}
                  </div>
                </div>
                <div className="mt-3">
                  <label className="flex items-center gap-2 text-xs font-bold text-foreground mb-2">
                    <MapPin className="w-3.5 h-3.5 text-muted-foreground" /> Localização no mapa
                  </label>
                  <MapPinPicker
                    latitude={latitude}
                    longitude={longitude}
                    onChange={({ latitude: lat, longitude: lng }) => {
                      setValue("address.latitude", lat, { shouldDirty: true });
                      setValue("address.longitude", lng, { shouldDirty: true });
                    }}
                    addressQuery={[
                      addressForGeocode?.street,
                      addressForGeocode?.number,
                      addressForGeocode?.neighborhood,
                      addressForGeocode?.city,
                      addressForGeocode?.stateCode,
                    ]
                      .filter(Boolean)
                      .join(", ")}
                  />
                </div>
              </div>

              <div>
                <label className="flex items-center gap-2 text-sm font-bold text-foreground mb-2">
                  <DollarSign className="w-4 h-4 text-muted-foreground" /> Valor (R$)
                </label>
                <Controller
                  control={control}
                  name="price"
                  rules={{ min: 0 }}
                  render={({ field }) => (
                    <MoneyInput
                      ref={field.ref}
                      name={field.name}
                      onBlur={field.onBlur}
                      fractionDigits={0}
                      withSymbol
                      value={typeof field.value === "number" ? field.value : undefined}
                      onChange={(v) => field.onChange(v ?? 0)}
                      className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                    />
                  )}
                />
              </div>

              <div>
                <label className="flex items-center gap-2 text-sm font-bold text-foreground mb-2">
                  <Building2 className="w-4 h-4 text-muted-foreground" /> Tipo
                </label>
                <select
                  {...register("type")}
                  className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                >
                  {PROPERTY_TYPES.map((t) => (
                    <option key={t} value={t}>
                      {t}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="flex items-center gap-2 text-sm font-bold text-foreground mb-2">
                  Status
                </label>
                <select
                  {...register("status")}
                  className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                >
                  {PROPERTY_STATUSES.map((s) => (
                    <option key={s} value={s}>
                      {s}
                    </option>
                  ))}
                </select>
              </div>

              <div className="grid grid-cols-2 md:grid-cols-5 gap-4 md:col-span-2">
                <div>
                  <label className="flex items-center gap-2 text-xs font-bold text-foreground mb-2">
                    <Bed className="w-3.5 h-3.5 text-muted-foreground" /> Quartos
                  </label>
                  <input
                    type="number"
                    {...register("beds", { valueAsNumber: true, min: 0 })}
                    className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                  />
                </div>
                <div>
                  <label className="flex items-center gap-2 text-xs font-bold text-foreground mb-2">
                    <Bed className="w-3.5 h-3.5 text-muted-foreground" /> Suítes
                  </label>
                  <input
                    type="number"
                    {...register("suites", { valueAsNumber: true, min: 0 })}
                    className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                  />
                </div>
                <div>
                  <label className="flex items-center gap-2 text-xs font-bold text-foreground mb-2">
                    <Bath className="w-3.5 h-3.5 text-muted-foreground" /> Banheiros
                  </label>
                  <input
                    type="number"
                    {...register("baths", { valueAsNumber: true, min: 0 })}
                    className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                  />
                </div>
                <div>
                  <label className="flex items-center gap-2 text-xs font-bold text-foreground mb-2">
                    <Square className="w-3.5 h-3.5 text-muted-foreground" /> Vagas
                  </label>
                  <input
                    type="number"
                    {...register("parkingSpaces", { valueAsNumber: true, min: 0 })}
                    className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                  />
                </div>
                <div>
                  <label className="flex items-center gap-2 text-xs font-bold text-foreground mb-2">
                    <Square className="w-3.5 h-3.5 text-muted-foreground" /> Área (m²)
                  </label>
                  <input
                    type="number"
                    {...register("area", { valueAsNumber: true, min: 0 })}
                    className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4 md:col-span-2">
                <div>
                  <label className="flex items-center gap-2 text-xs font-bold text-foreground mb-2">
                    Condomínio (R$)
                  </label>
                  <Controller
                    control={control}
                    name="condominiumFee"
                    rules={{ min: 0 }}
                    render={({ field }) => (
                      <MoneyInput
                        ref={field.ref}
                        name={field.name}
                        onBlur={field.onBlur}
                        fractionDigits={0}
                        value={typeof field.value === "number" ? field.value : undefined}
                        onChange={(v) => field.onChange(v ?? 0)}
                        className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                      />
                    )}
                  />
                </div>
                <div>
                  <label className="flex items-center gap-2 text-xs font-bold text-foreground mb-2">
                    IPTU (R$)
                  </label>
                  <Controller
                    control={control}
                    name="propertyTax"
                    rules={{ min: 0 }}
                    render={({ field }) => (
                      <MoneyInput
                        ref={field.ref}
                        name={field.name}
                        onBlur={field.onBlur}
                        fractionDigits={0}
                        value={typeof field.value === "number" ? field.value : undefined}
                        onChange={(v) => field.onChange(v ?? 0)}
                        className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                      />
                    )}
                  />
                </div>
              </div>

              <div className="md:col-span-2">
                <label className="flex items-center gap-2 text-sm font-bold text-foreground mb-2">
                  Descrição
                </label>
                <textarea
                  {...register("description")}
                  className="w-full h-24 bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium resize-none"
                  placeholder="Descrição detalhada do imóvel..."
                />
              </div>

              <div className="md:col-span-2">
                <label htmlFor="property-amenities" className="flex items-center gap-2 text-sm font-bold text-foreground mb-2">
                  Comodidades
                </label>
                <TagInput
                  id="property-amenities"
                  label="Comodidades"
                  value={watch("amenities") || []}
                  onChange={(next) => setValue("amenities", next, { shouldDirty: true })}
                  suggestions={AMENITY_SUGGESTIONS}
                  placeholder="Ex: Piscina, Academia, Salão de Festas"
                  hint="Digite e confirme com Enter ou vírgula. Dá para colar a lista inteira separada por vírgulas."
                />
              </div>

              <div className="md:col-span-2">
                <PropertyMediaSection
                  control={control}
                  register={register}
                  watch={watch}
                  setValue={setValue}
                  getValues={getValues}
                  propertyId={property?.id}
                  onAddPendingFile={(pendingId, file) =>
                    pendingFilesRef.current.set(pendingId, file)
                  }
                  onRemovePendingFile={(pendingId) => pendingFilesRef.current.delete(pendingId)}
                  onMarkDeleteFilename={(filename) => pendingDeletesRef.current.push(filename)}
                  onImagesLoadingChange={setImagesLoadingCount}
                  onImagesErrorChange={setImagesHaveErrors}
                />
              </div>
            </div>
          </div>

          <div className="space-y-4">
            <h3 className="text-sm font-bold text-foreground border-b border-border pb-2 mb-4 flex items-center gap-2">
              <Briefcase className="w-4 h-4 text-primary-500 dark:text-primary-400" /> Negócio e Propriedade
            </h3>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div className="md:col-span-2">
                <label className="block text-sm font-bold text-foreground mb-2">
                  Disponível para
                </label>
                <div className="flex gap-4">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      value="sale"
                      {...register("availableFor")}
                      className="rounded border-border bg-card text-primary-600 focus:ring-primary-500"
                    />
                    <span className="text-sm text-foreground ">Venda</span>
                  </label>
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      value="rent"
                      {...register("availableFor")}
                      className="rounded border-border bg-card text-primary-600 focus:ring-primary-500"
                    />
                    <span className="text-sm text-foreground ">Locação</span>
                  </label>
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      value="exchange"
                      {...register("availableFor")}
                      className="rounded border-border bg-card text-primary-600 focus:ring-primary-500"
                    />
                    <span className="text-sm text-foreground ">Permuta</span>
                  </label>
                </div>
              </div>

              <div className="md:col-span-2">
                <label className="flex items-center gap-2 text-sm font-bold text-foreground mb-2">
                  Contrato de Intermediação (ID/Ref)
                </label>
                <input
                  {...register("intermediationContractId")}
                  className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                  placeholder="Ex: ADM-2024-001"
                />
              </div>

              <div className="md:col-span-2">
                <label className="block text-sm font-bold text-foreground mb-2">
                  Exclusividade da Captação
                </label>
                <div className="grid grid-cols-2 gap-2">
                  {([
                    { value: "open" as const, label: "Aberta", hint: "Outras imobiliárias podem captar" },
                    { value: "exclusive" as const, label: "Exclusiva", hint: "Captação exclusiva desta imobiliária" },
                  ]).map((opt) => (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => {
                        setValue("captureExclusivity", opt.value, { shouldDirty: true });
                        if (opt.value === "exclusive") {
                          setValue("partnershipPortfolio", false, { shouldDirty: true });
                        }
                      }}
                      className={clsx(
                        "px-3 py-2.5 rounded-xl border text-left transition-all",
                        captureExclusivity === opt.value
                          ? "bg-primary-50 border-primary-300 ring-2 ring-primary-600/20"
                          : "bg-muted border-border hover:bg-muted/70",
                      )}
                    >
                      <span className="block text-sm font-bold text-foreground">{opt.label}</span>
                      <span className="block text-[11px] text-muted-foreground mt-0.5">
                        {opt.hint}
                      </span>
                    </button>
                  ))}
                </div>
              </div>

              <div className="md:col-span-2">
                <div
                  className={clsx(
                    "flex items-start justify-between gap-4 p-4 rounded-xl border",
                    exclusivityLocksSharing ? "bg-muted/50 border-border" : "bg-muted border-border",
                  )}
                >
                  <div className="min-w-0">
                    <label htmlFor="partnership-portfolio-toggle" className="text-sm font-bold text-foreground">
                      Compartilhar com parceiros
                    </label>
                    <p className="text-[11px] text-muted-foreground mt-0.5">
                      {exclusivityLocksSharing
                        ? "Indisponível: imóveis com captação exclusiva não podem ser compartilhados."
                        : "Exibe este imóvel no portfólio compartilhado das imobiliárias parceiras."}
                    </p>
                  </div>
                  <Switch
                    id="partnership-portfolio-toggle"
                    checked={!!partnershipPortfolio && !exclusivityLocksSharing}
                    disabled={exclusivityLocksSharing}
                    onCheckedChange={(checked) =>
                      setValue("partnershipPortfolio", checked, { shouldDirty: true })
                    }
                  />
                </div>
              </div>

              <div className="md:col-span-2 space-y-3">
                <div className="flex items-center justify-between">
                  <label className="flex items-center gap-2 text-sm font-bold text-foreground ">
                    <User className="w-4 h-4 text-muted-foreground" /> Proprietários
                  </label>
                  <button
                    type="button"
                    onClick={() => appendOwner({ id: "", percentage: 100 })}
                    className="text-xs flex items-center gap-1 text-primary-600 dark:text-primary-400 hover:text-primary-700 dark:hover:text-primary-300 font-medium"
                  >
                    <Plus className="w-3 h-3" /> Adicionar
                  </button>
                </div>

                {ownerFields.map((field, index) => {
                  const currentOwnerId = watch(`owners.${index}.id`);
                  const selectedOwner = leads.find((l) => l.id === currentOwnerId);

                  return (
                    <div key={field.id} className="flex gap-2 items-start">
                      <div className="flex-1">
                        <Popover
                          open={openOwnerCombobox[field.id]}
                          onOpenChange={(open) =>
                            setOpenOwnerCombobox((prev) => ({ ...prev, [field.id]: open }))
                          }
                        >
                          <PopoverTrigger asChild>
                            <button
                              type="button"
                              className="w-full flex items-center justify-between px-3 py-2.5 bg-muted border border-border rounded-xl text-sm text-foreground outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all"
                            >
                              {selectedOwner ? (
                                <span className="truncate">
                                  {selectedOwner.name}{" "}
                                  {selectedOwner.isClient ? "(Cliente)" : "(Lead)"}
                                </span>
                              ) : (
                                <span className="text-muted-foreground">Selecione um cliente...</span>
                              )}
                              <ChevronsUpDown className="w-4 h-4 text-muted-foreground opacity-50 shrink-0" />
                            </button>
                          </PopoverTrigger>
                          <PopoverContent
                            className="w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50"
                            align="start"
                          >
                            <Command>
                              <CommandInput placeholder="Buscar cliente..." />
                              <CommandList>
                                <CommandEmpty>Nenhum cliente encontrado.</CommandEmpty>
                                <CommandGroup>
                                  {leads.map((lead) => (
                                    <CommandItem
                                      key={lead.id}
                                      value={lead.name}
                                      onSelect={() => {
                                        setValue(`owners.${index}.id`, lead.id, {
                                          shouldValidate: true,
                                          shouldDirty: true,
                                        });
                                        setOpenOwnerCombobox((prev) => ({
                                          ...prev,
                                          [field.id]: false,
                                        }));
                                      }}
                                      className="cursor-pointer"
                                    >
                                      <Check
                                        className={clsx(
                                          "w-4 h-4 mr-2",
                                          currentOwnerId === lead.id ? "opacity-100" : "opacity-0",
                                        )}
                                      />
                                      <span>
                                        {lead.name}{" "}
                                        <span className="text-xs text-muted-foreground ml-1">
                                          {lead.isClient ? "(Cliente)" : "(Lead)"}
                                        </span>
                                      </span>
                                    </CommandItem>
                                  ))}
                                </CommandGroup>
                              </CommandList>
                            </Command>
                          </PopoverContent>
                        </Popover>
                        <input
                          type="hidden"
                          {...register(`owners.${index}.id` as const, { required: true })}
                        />
                      </div>
                      <div className="w-24">
                        <input
                          type="number"
                          placeholder="%"
                          {...register(`owners.${index}.percentage` as const, { valueAsNumber: true })}
                          className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500"
                        />
                      </div>
                      <button
                        type="button"
                        onClick={() => removeOwner(index)}
                        className="p-2.5 text-muted-foreground hover:text-status-critical transition-colors"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  );
                })}
                {ownerFields.length === 0 && (
                  <p className="text-xs text-muted-foreground italic">Nenhum proprietário vinculado.</p>
                )}
              </div>

              <div className="md:col-span-2 space-y-3">
                <div className="flex items-center justify-between">
                  <label className="flex items-center gap-2 text-sm font-bold text-foreground ">
                    <User className="w-4 h-4 text-muted-foreground" /> Corretores Captadores
                  </label>
                  <button
                    type="button"
                    onClick={() => appendBroker({ id: "", role: "captador" })}
                    className="text-xs flex items-center gap-1 text-primary-600 dark:text-primary-400 hover:text-primary-700 dark:hover:text-primary-300 font-medium"
                  >
                    <Plus className="w-3 h-3" /> Adicionar
                  </button>
                </div>

                {brokerFields.map((field, index) => {
                  const currentBrokerId = watch(`capturingBrokers.${index}.id`);
                  const selectedBroker = displayAgents.find((a) => a.id === currentBrokerId);

                  return (
                    <div key={field.id} className="flex gap-2 items-start">
                      <div className="flex-1">
                        <Popover
                          open={openBrokerCombobox[field.id]}
                          onOpenChange={(open) =>
                            setOpenBrokerCombobox((prev) => ({ ...prev, [field.id]: open }))
                          }
                        >
                          <PopoverTrigger asChild>
                            <button
                              type="button"
                              className="w-full flex items-center justify-between px-3 py-2.5 bg-muted border border-border rounded-xl text-sm text-foreground outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all"
                            >
                              {selectedBroker ? (
                                <div className="flex items-center gap-2 truncate">
                                  <Avatar className="w-5 h-5 border border-border">
                                    <AvatarImage src={selectedBroker.avatar} alt={selectedBroker.name} />
                                    <AvatarFallback className="text-caption font-bold">
                                      {selectedBroker.name.charAt(0).toUpperCase()}
                                    </AvatarFallback>
                                  </Avatar>
                                  <span className="truncate">{selectedBroker.name}</span>
                                </div>
                              ) : (
                                <span className="text-muted-foreground">Selecione um corretor...</span>
                              )}
                              <ChevronsUpDown className="w-4 h-4 text-muted-foreground opacity-50 shrink-0" />
                            </button>
                          </PopoverTrigger>
                          <PopoverContent
                            className="w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50"
                            align="start"
                          >
                            <Command>
                              <CommandInput placeholder="Buscar corretor..." />
                              <CommandList>
                                <CommandEmpty>Nenhum corretor encontrado.</CommandEmpty>
                                <CommandGroup>
                                  {displayAgents.map((agent) => (
                                    <CommandItem
                                      key={agent.id}
                                      value={agent.name}
                                      onSelect={() => {
                                        setValue(`capturingBrokers.${index}.id`, agent.id, {
                                          shouldValidate: true,
                                          shouldDirty: true,
                                        });
                                        setOpenBrokerCombobox((prev) => ({
                                          ...prev,
                                          [field.id]: false,
                                        }));
                                      }}
                                      className="cursor-pointer"
                                    >
                                      <Check
                                        className={clsx(
                                          "w-4 h-4 mr-2",
                                          currentBrokerId === agent.id ? "opacity-100" : "opacity-0",
                                        )}
                                      />
                                      <div className="flex items-center gap-2">
                                        <Avatar className="w-5 h-5 border border-border">
                                          <AvatarImage src={agent.avatar} alt={agent.name} />
                                          <AvatarFallback className="text-caption font-bold">
                                            {agent.name.charAt(0).toUpperCase()}
                                          </AvatarFallback>
                                        </Avatar>
                                        <span>{agent.name}</span>
                                      </div>
                                    </CommandItem>
                                  ))}
                                </CommandGroup>
                              </CommandList>
                            </Command>
                          </PopoverContent>
                        </Popover>
                        <input
                          type="hidden"
                          {...register(`capturingBrokers.${index}.id` as const, { required: true })}
                        />
                      </div>
                      <div className="w-32">
                        <select
                          {...register(`capturingBrokers.${index}.role` as const)}
                          className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500"
                        >
                          <option value="captador">Captador</option>
                          <option value="co_captador">Co-Captador</option>
                        </select>
                      </div>
                      <button
                        type="button"
                        onClick={() => removeBroker(index)}
                        className="p-2.5 text-muted-foreground hover:text-status-critical transition-colors"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  );
                })}
                {brokerFields.length === 0 && (
                  <p className="text-xs text-muted-foreground italic">Nenhum corretor vinculado.</p>
                )}
              </div>
            </div>
          </div>
        </form>
      </div>

      <div className="flex-none px-6 py-4 border-t border-border bg-muted /50 flex items-center justify-end gap-3 shrink-0">
        {imagesLoadingCount > 0 && (
          <span className="mr-auto text-xs text-muted-foreground flex items-center gap-1.5">
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
            Aguardando {imagesLoadingCount} imagem(ns) carregar…
          </span>
        )}
        <Button
          type="button"
          onClick={onClose}
          variant="ghost"
          disabled={isSubmitting}
          className="px-5 text-sm font-bold text-muted-foreground bg-muted"
        >
          Cancelar
        </Button>
        <Button
          type="submit"
          form="property-form"
          loading={isSubmitting}
          loadingText={property ? "Salvando…" : "Criando…"}
          disabled={imagesLoadingCount > 0}
          className="px-5 text-sm font-bold bg-primary-600 hover:bg-primary-700 shadow-sm"
        >
          {property ? "Salvar Alterações" : "Criar Imóvel"}
        </Button>
      </div>
    </div>
  );
}
