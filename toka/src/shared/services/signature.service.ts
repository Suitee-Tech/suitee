import api, { unwrapApiData } from "./api";
import type { SignatureEnvelope } from "../contracts/types";

export const signatureService = {
  getByContract: async (contractId: string): Promise<SignatureEnvelope | null> => {
    try {
      const response = await api.get<any>(`/signature/envelopes/by-contract/${contractId}`);
      return unwrapApiData<SignatureEnvelope>(response);
    } catch (e: any) {
      if (e?.response?.status === 404) return null;
      throw e;
    }
  },

  getById: async (id: string): Promise<SignatureEnvelope> => {
    const response = await api.get<any>(`/signature/envelopes/${id}`);
    return unwrapApiData<SignatureEnvelope>(response);
  },

  list: async (): Promise<SignatureEnvelope[]> => {
    const response = await api.get<any>("/signature/envelopes");
    return unwrapApiData<SignatureEnvelope[]>(response) || [];
  },

  cancel: async (envelopeId: string): Promise<SignatureEnvelope> => {
    const response = await api.post<any>(`/signature/envelopes/${envelopeId}/cancel`);
    return unwrapApiData<SignatureEnvelope>(response)!;
  },

  resend: async (envelopeId: string, signerId?: string): Promise<SignatureEnvelope> => {
    const query = signerId ? `?signer_id=${encodeURIComponent(signerId)}` : "";
    const response = await api.post<any>(`/signature/envelopes/${envelopeId}/resend${query}`);
    return unwrapApiData<SignatureEnvelope>(response)!;
  },

  reconcile: async (envelopeId: string): Promise<SignatureEnvelope> => {
    const response = await api.post<any>(`/signature/envelopes/${envelopeId}/reconcile`);
    return unwrapApiData<SignatureEnvelope>(response)!;
  },

  editSigner: async (
    envelopeId: string,
    signerId: string,
    payload: {
      name: string;
      email: string;
      phone_number?: string;
      documentation?: string;
      birthday?: string;
    },
  ): Promise<SignatureEnvelope> => {
    const response = await api.patch<any>(`/signature/envelopes/${envelopeId}/signers/${signerId}`, payload);
    return unwrapApiData<SignatureEnvelope>(response)!;
  },

  addSigner: async (
    envelopeId: string,
    payload: {
      name: string;
      email: string;
      phone_number?: string;
      documentation?: string;
      birthday?: string;
      role?: string;
      auth?: string[];
    },
  ): Promise<SignatureEnvelope> => {
    const response = await api.post<any>(`/signature/envelopes/${envelopeId}/signers`, payload);
    return unwrapApiData<SignatureEnvelope>(response)!;
  },
};

