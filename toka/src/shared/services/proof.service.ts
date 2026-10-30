import api, { API_URL } from './api';

export type ProofKind = 'payout' | 'charge' | 'commission' | 'transaction';

export interface ProofUploadResponse {
  url: string;
  object_key?: string;
  filename: string;
  size_bytes: number;
  expires_at?: string;
}

export interface ProofDownloadResponse {
  blob: Blob;
  contentType: string;
  filename?: string;
}

const INTERNAL_CONTENT_PREFIX = '/api/v1/proofs/content/';
const STORAGE_OBJECT_PREFIX = '/storage/v1/object/';
const LEGACY_UPLOAD_PREFIX = '/uploads/financial-proofs/';

const encodeObjectKey = (objectKey: string): string =>
  btoa(objectKey).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');

const filenameFromContentDisposition = (header: string | undefined): string | undefined => {
  if (!header) return undefined;
  const utf8 = header.match(/filename\*\s*=\s*UTF-8''([^;]+)/i)?.[1];
  if (utf8) {
    try {
      return decodeURIComponent(utf8.replace(/^"|"$/g, ''));
    } catch {
      return utf8.replace(/^"|"$/g, '');
    }
  }
  return header.match(/filename\s*=\s*"?([^";]+)"?/i)?.[1]?.trim();
};

/**
 * Converts the current proof representation into an API-relative route.
 * Newly uploaded proofs already contain this route; legacy Supabase signed
 * URLs are converted by extracting only the object key, never forwarding the
 * signed token to the API or storing it in another browser URL.
 */
export const getProofContentPath = (rawUrl: string): string | null => {
  const raw = rawUrl.trim();
  if (!raw || typeof window === 'undefined') return null;

  try {
    const parsed = new URL(raw, window.location.origin);
    const internalIndex = parsed.pathname.indexOf(INTERNAL_CONTENT_PREFIX);
    if (internalIndex >= 0) {
      return parsed.pathname.slice(internalIndex + '/api/v1'.length);
    }

    const storageIndex = parsed.pathname.indexOf(STORAGE_OBJECT_PREFIX);
    let objectKey = '';
    if (storageIndex >= 0) {
      const storageParts = parsed.pathname
        .slice(storageIndex + STORAGE_OBJECT_PREFIX.length)
        .split('/')
        .filter(Boolean);
      if (storageParts[0] !== 'sign' && storageParts[0] !== 'authenticated') return null;
      if (storageParts[1] !== 'financial-proofs') return null;
      objectKey = storageParts.slice(2).map((part) => decodeURIComponent(part)).join('/');
    } else if (parsed.pathname.startsWith(LEGACY_UPLOAD_PREFIX)) {
      objectKey = parsed.pathname
        .slice(LEGACY_UPLOAD_PREFIX.length)
        .split('/')
        .filter(Boolean)
        .map((part) => decodeURIComponent(part))
        .join('/');
    }
    if (!objectKey || objectKey.split('/').length !== 4) return null;

    return `/proofs/content/${encodeObjectKey(objectKey)}`;
  } catch {
    return null;
  }
};

export const getProofBrowserURL = (contentPath: string): string => {
  if (typeof window === 'undefined') return contentPath;
  const apiURL = new URL(API_URL, window.location.origin);
  const apiBasePath = apiURL.pathname.replace(/\/+$/, '');
  return new URL(`${apiBasePath}${contentPath}`, apiURL.origin).toString();
};

class ProofService {
  private readonly baseUrl = '/proofs';

  async upload(file: File, kind: ProofKind): Promise<ProofUploadResponse> {
    const form = new FormData();
    form.append('file', file);
    form.append('kind', kind);
    const res = await api.post<{ data: ProofUploadResponse }>(this.baseUrl, form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
    return res.data?.data;
  }

  async download(rawUrl: string): Promise<ProofDownloadResponse> {
    const path = getProofContentPath(rawUrl);
    if (!path) {
      throw new Error('proof is not managed by the financial storage');
    }

    const response = await api.get<Blob>(path, { responseType: 'blob' });
    const contentType = response.headers['content-type'];
    const contentDisposition = response.headers['content-disposition'];
    return {
      blob: response.data,
      contentType: typeof contentType === 'string' ? contentType : 'application/octet-stream',
      filename: typeof contentDisposition === 'string'
        ? filenameFromContentDisposition(contentDisposition)
        : undefined,
    };
  }
}

export const proofService = new ProofService();
