import { createRequestId } from "../lib/requestId";
import axios, { type AxiosError, type AxiosInstance, type AxiosRequestConfig } from "axios";

export type RetryableAxiosRequestConfig = AxiosRequestConfig & {
  _retry?: boolean;
  _silent?: boolean;
};

export function unwrapData<T>(input: any): T {
  if (input && typeof input === "object" && "data" in input) {
    if (input.data && typeof input.data === "object" && "data" in input.data) {
      return input.data.data;
    }
    return input.data;
  }
  return input;
}

export function createRefreshDedupe<T>(refresh: () => Promise<T>) {
  let promise: Promise<T> | null = null;
  return () => {
    if (!promise) {
      promise = refresh().finally(() => {
        promise = null;
      });
    }
    return promise;
  };
}

export function createHttpClient(options: {
  baseURL: string;
  withCredentials?: boolean;
  timeout?: number;
  getAccessToken: () => string | null;
  setAccessToken: (token: string | null) => void;
  shouldAttachToken?: (config: AxiosRequestConfig) => boolean;
  getExtraHeaders?: () => Record<string, string | undefined> | null | undefined;
  shouldRefresh: (params: {
    error: AxiosError;
    originalRequest: RetryableAxiosRequestConfig;
  }) => boolean;
  refreshAccessToken: (params: { client: AxiosInstance }) => Promise<string>;
  onRefreshFailure: (params: {
    error: unknown;
    originalRequest: RetryableAxiosRequestConfig;
  }) => void;
}) {
  const client = axios.create({
    baseURL: options.baseURL,
    withCredentials: options.withCredentials ?? true,
    timeout: options.timeout,
  });

  client.interceptors.request.use((config) => {
    const shouldAttach = options.shouldAttachToken ? options.shouldAttachToken(config) : true;
    if (shouldAttach) {
      const token = options.getAccessToken();
      if (token) {
        config.headers = config.headers ?? {};
        config.headers.Authorization = `Bearer ${token}`;
      }
    }

    const extra = options.getExtraHeaders?.();
    if (extra) {
      config.headers = config.headers ?? {};
      for (const [k, v] of Object.entries(extra)) {
        if (typeof v === "string" && v.length > 0) {
          config.headers[k] = v;
        }
      }
    }

    if (!config.headers.has("X-Request-ID")) config.headers.set("X-Request-ID", createRequestId());
    return config;
  });

  client.interceptors.response.use(
    (response) => response,
    async (error: AxiosError) => {
      const originalRequest = error.config as RetryableAxiosRequestConfig | undefined;
      if (!originalRequest) return Promise.reject(error);

      if (!options.shouldRefresh({ error, originalRequest })) {
        return Promise.reject(error);
      }

      originalRequest._retry = true;

      try {
        const newToken = await options.refreshAccessToken({ client });
        options.setAccessToken(newToken);
        originalRequest.headers = originalRequest.headers ?? {};
        originalRequest.headers.Authorization = `Bearer ${newToken}`;
        return client(originalRequest);
      } catch (err) {
        options.onRefreshFailure({ error: err, originalRequest });
        return Promise.reject(err);
      }
    },
  );

  return client;
}

