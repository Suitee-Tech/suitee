import api from "./api";

export function isInspectorService(serviceType: string): boolean {
  return /^(vistoria|vistorias|vistoriador|vistoriadora)$/i.test(serviceType.trim());
}

/**
 * Tipo de prestador: externo (terceirizado) ou interno (equipe da imobiliária).
 */
export type SupplierKind = "externo" | "interno";

/**
 * Profissional/fornecedor cadastrado pela imobiliária. É reutilizado em
 * chamados de manutenção e vistorias — as características, o contato e o tipo
 * de serviço ficam no cadastro, não no chamado.
 */
export interface Supplier {
  id: string;
  tenant_id: string;
  name: string;
  service_type: string;
  kind: SupplierKind;
  document?: string;
  email?: string;
  phone?: string;
  notes?: string;
  city?: string;
  active: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateSupplierDTO {
  name: string;
  service_type: string;
  kind?: SupplierKind;
  document?: string;
  email?: string;
  phone?: string;
  notes?: string;
  city?: string;
}

export interface UpdateSupplierDTO extends Partial<CreateSupplierDTO> {
  active?: boolean;
}

/** Rótulo de seleção: "Carlos Silva (Elétrica)". */
export function supplierDisplayName(supplier: Pick<Supplier, "name" | "service_type">): string {
  return supplier.service_type ? `${supplier.name} (${supplier.service_type})` : supplier.name;
}

/** Contato disponível, na ordem em que o operador costuma acionar. */
export function supplierContact(supplier: Pick<Supplier, "phone" | "email">): string {
  return supplier.phone || supplier.email || "";
}

class SupplierService {
  private readonly baseUrl = "/suppliers";

  async getAll(params?: {
    q?: string;
    service_type?: string;
    active_only?: boolean;
    limit?: number;
    offset?: number;
  }) {
    const response = await api.get<{ data: Supplier[]; meta: { total?: number; limit?: number } }>(this.baseUrl, { params });
    return response.data;
  }

  async getById(id: string) {
    const response = await api.get<{ data: Supplier }>(`${this.baseUrl}/${id}`);
    return response.data;
  }

  /** Tipos de serviço já usados pela imobiliária, para não inventar sinônimos. */
  async getServiceTypes() {
    const response = await api.get<{ data: string[] }>(`${this.baseUrl}/service-types`);
    return response.data;
  }

  async create(data: CreateSupplierDTO) {
    const response = await api.post<{ data: Supplier }>(this.baseUrl, data);
    return response.data;
  }

  async update(id: string, data: UpdateSupplierDTO) {
    const response = await api.put<{ data: Supplier }>(`${this.baseUrl}/${id}`, data);
    return response.data;
  }

  /** Tira o prestador da seleção sem apagar o histórico dos chamados. */
  async deactivate(id: string) {
    return this.update(id, { active: false });
  }

  async delete(id: string) {
    const response = await api.delete(`${this.baseUrl}/${id}`);
    return response.data;
  }
}

export const supplierService = new SupplierService();
