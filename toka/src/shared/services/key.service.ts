import api from './api';

export interface Key {
 id: string;
 tenant_id: string;
 property_id: string;
 code: string;
 location: string;
 status: string;
 checked_out_by?: string;
 checked_out_at?: string;
 returned_at?: string;
 notes?: string;
 created_at: string;
 updated_at: string;
}

export interface CreateKeyDTO {
 property_id: string;
 code: string;
 location: string;
 notes?: string;
}

class KeyService {
 private readonly baseUrl = '/keys';

 async getAll(params?: { property_id?: string; status?: string; limit?: number; offset?: number }) {
 const response = await api.get<{ data: Key[]; meta: any }>(this.baseUrl, { params });
 return response.data;
 }

 async getById(id: string) {
 const response = await api.get<{ data: Key }>(`${this.baseUrl}/${id}`);
 return response.data;
 }

 async create(data: CreateKeyDTO) {
 const response = await api.post<{ data: Key }>(this.baseUrl, data);
 return response.data;
 }

 async delete(id: string) {
 return api.delete(`${this.baseUrl}/${id}`);
 }

 async checkout(id: string, checkedOutBy: string) {
 const response = await api.post<{ data: Key }>(`${this.baseUrl}/${id}/checkout`, { checked_out_by: checkedOutBy });
 return response.data;
 }

 async checkin(id: string) {
 const response = await api.post<{ data: Key }>(`${this.baseUrl}/${id}/checkin`, {});
 return response.data;
 }
}

export const keyService = new KeyService();

