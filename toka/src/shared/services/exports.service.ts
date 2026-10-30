import api, { unwrapApiData } from "./api"

export type ExportStatus = "QUEUED" | "RUNNING" | "SUCCEEDED" | "FAILED"
export type ExportFormat = "csv" | "pdf"

export interface ExportJob {
  id: string
  tenant_id: string
  requested_by: string
  entity_type: string
  format: ExportFormat
  status: ExportStatus
  filters?: Record<string, unknown>
  result_url?: string
  result_bucket?: string
  result_path?: string
  row_count?: number
  error_message?: string
  created_at: string
  started_at?: string
  completed_at?: string
}

export interface CreateLeadExportRequest {
  format: ExportFormat
  filters?: Record<string, unknown>
}

export const exportsService = {
  createLeadExport: async (req: CreateLeadExportRequest): Promise<ExportJob> => {
    const response = await api.post<{ data: ExportJob }>("/leads/export", req)
    return unwrapApiData<ExportJob>(response)
  },

  getJob: async (id: string): Promise<ExportJob> => {
    const response = await api.get<{ data: ExportJob }>(`/exports/${id}`)
    return unwrapApiData<ExportJob>(response)
  },

  listMine: async (limit = 50, offset = 0): Promise<ExportJob[]> => {
    const response = await api.get<{ data: ExportJob[] }>("/exports/mine", {
      params: { limit, offset },
    })
    return unwrapApiData<ExportJob[]>(response) ?? []
  },

  pollUntilDone: async (
    id: string,
    opts?: { pollEveryMs?: number; timeoutMs?: number; onProgress?: (j: ExportJob) => void },
  ): Promise<ExportJob> => {
    const pollEvery = opts?.pollEveryMs ?? 2000
    const timeout = opts?.timeoutMs ?? 5 * 60 * 1000
    const start = Date.now()
    while (Date.now() - start < timeout) {
      const j = await exportsService.getJob(id)
      opts?.onProgress?.(j)
      if (j.status === "SUCCEEDED" || j.status === "FAILED") {
        return j
      }
      await new Promise((r) => setTimeout(r, pollEvery))
    }
    throw new Error('Export timeout — tente novamente em "Meus exports"')
  },
}
