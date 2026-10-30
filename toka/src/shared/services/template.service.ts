import api, { unwrapApiData } from "./api";

export interface ContractTemplate {
  id: string;
  name: string;
  description: string;
  content: string;
  variables: string[];
  created_at: string;
  updated_at: string;
}

export interface CreateTemplateRequest {
  name: string;
  description?: string;
  content: string;
  variables?: string[];
}

class TemplateService {
  async list(): Promise<ContractTemplate[]> {
    const response = await api.get("/contract-templates");
    const data = unwrapApiData(response);
    return Array.isArray(data) ? data : [];
  }

  async getById(id: string): Promise<ContractTemplate> {
    const response = await api.get(`/contract-templates/${id}`);
    return unwrapApiData(response);
  }

  async create(data: CreateTemplateRequest): Promise<ContractTemplate> {
    const response = await api.post("/contract-templates", data);
    return unwrapApiData(response);
  }

  async update(id: string, data: CreateTemplateRequest): Promise<ContractTemplate> {
    const response = await api.put(`/contract-templates/${id}`, data);
    return unwrapApiData(response);
  }

  async delete(id: string): Promise<void> {
    await api.delete(`/contract-templates/${id}`);
  }
}

export const templateService = new TemplateService();
