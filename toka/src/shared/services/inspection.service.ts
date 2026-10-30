import api from './api';

export interface Finding {
 id?: string;
 room: string;
 description: string;
 condition: string;
 notes?: string;
 photos?: string[];
 thumbnails?: string[];
}

export interface Inspection {
 id: string;
 tenant_id: string;
 property_id: string;
 contract_id: string;
 type: string;
 scheduled_date: string;
 completed_date?: string;
 inspector_id: string;
 inspector_name: string;
 status: string;
 findings?: Finding[];
 notes?: string;
 report_url?: string;
 created_at: string;
 updated_at: string;
}

export interface CreateInspectionDTO {
 time_zone?: string;
 property_id: string;
 contract_id: string;
 type: string;
 scheduled_date: string;
 inspector_id?: string;
 inspector_name?: string;
 notes?: string;
}

export interface UpdateInspectionDTO extends Partial<CreateInspectionDTO> {
 status?: string;
 findings?: Finding[];
 completed_date?: string;
 report_url?: string;
}

export interface Deduction {
 room: string;
 description: string;
 amount: number;
 evidence_photos?: string[];
}

class InspectionService {
 private readonly baseUrl = '/inspections';

 async getAll(params?: { property_id?: string; contract_id?: string; status?: string; limit?: number; offset?: number }) {
 const response = await api.get<{ data: Inspection[]; meta: any }>(this.baseUrl, { params });
 return response.data;
 }

 async getById(id: string) {
 const response = await api.get<{ data: Inspection }>(`${this.baseUrl}/${id}`);
 return response.data;
 }

 async create(data: CreateInspectionDTO) {
 const response = await api.post<{ data: Inspection }>(this.baseUrl, data);
 return response.data;
 }

 async update(id: string, data: UpdateInspectionDTO) {
 const response = await api.put<{ data: Inspection }>(`${this.baseUrl}/${id}`, data);
 return response.data;
 }

 async delete(id: string) {
 return api.delete(`${this.baseUrl}/${id}`);
 }

 async recordDeductions(id: string, deductions: Deduction[]) {
 const response = await api.post<{ data: Inspection }>(`${this.baseUrl}/${id}/deductions`, { deductions });
 return response.data;
 }

 async uploadFindingPhotos(
   inspectionId: string,
   findingId: string,
   files: File[],
 ): Promise<{ urls: string[]; thumbnails: string[]; uploaded: number }> {
  const formData = new FormData();
  for (const f of files) {
   formData.append("files", f, f.name);
  }
  const response = await api.post<{ data: { urls: string[]; thumb_urls?: string[]; uploaded: number; inspection_id: string; finding_id: string } }>(
   `${this.baseUrl}/${inspectionId}/findings/${findingId}/photos`,
   formData,
   { headers: { "Content-Type": "multipart/form-data" } },
  );
  const urls = response.data?.data?.urls || [];
  const thumbnails = response.data?.data?.thumb_urls || urls.map(() => "");
  return {
   urls,
   thumbnails,
   uploaded: response.data?.data?.uploaded || 0,
  };
 }
}

export const inspectionService = new InspectionService();

