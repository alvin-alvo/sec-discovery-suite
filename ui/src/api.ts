import {
  Run,
  AgentStatus,
  Schedule,
  FSSummary,
  FSCertificateItem,
  FSFacets,
  TLSSummary,
  TLSObservationItem,
  TLSEndpointGroupItem,
  TLSEvidence,
  GraphData,
} from './types'

async function fetchJSON<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, options)
  if (!res.ok) {
    let msg = `HTTP error ${res.status}`
    try {
      const errObj = await res.json()
      if (errObj?.error?.message) {
        msg = errObj.error.message
      }
    } catch {
      // ignore
    }
    throw new Error(msg)
  }
  return res.json()
}

export const api = {
  getAgentStatus: () => fetchJSON<AgentStatus>('/api/v1/agent/status'),

  getRuns: (params?: { type?: string; status?: string; limit?: number }) => {
    const q = new URLSearchParams()
    if (params?.type) q.set('type', params.type)
    if (params?.status) q.set('status', params.status)
    if (params?.limit) q.set('limit', String(params.limit))
    return fetchJSON<{ runs: Run[] }>(`/api/v1/runs?${q.toString()}`)
  },

  getRun: (id: string) => fetchJSON<Run>(`/api/v1/runs/${id}`),

  createRun: (type: 'filesystem' | 'tls', params?: any) =>
    fetchJSON<{ run_id: string; status: string }>('/api/v1/runs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type, params }),
    }),

  getSchedules: () =>
    fetchJSON<{ schedules: Schedule[] }>('/api/v1/schedules'),

  updateSchedule: (schedule: Schedule) =>
    fetchJSON<{ status: string }>('/api/v1/schedules', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(schedule),
    }),

  getFSSummary: (runId: string) =>
    fetchJSON<FSSummary>(`/api/v1/runs/${runId}/fs/summary`),

  getFSFacets: (runId: string) =>
    fetchJSON<FSFacets>(`/api/v1/runs/${runId}/fs/facets`),

  getFSCertificates: (runId: string, params: {
    q?: string
    status?: string
    issuer?: string
    algo?: string
    format?: string
    parse_status?: string
    sort?: string
    limit?: number
    offset?: number
  }) => {
    const q = new URLSearchParams()
    if (params.q) q.set('q', params.q)
    if (params.status) q.set('status', params.status)
    if (params.issuer) q.set('issuer', params.issuer)
    if (params.algo) q.set('algo', params.algo)
    if (params.format) q.set('format', params.format)
    if (params.parse_status) q.set('parse_status', params.parse_status)
    if (params.sort) q.set('sort', params.sort)
    if (params.limit !== undefined) q.set('limit', String(params.limit))
    if (params.offset !== undefined) q.set('offset', String(params.offset))
    return fetchJSON<{ items: FSCertificateItem[]; total: number; limit: number; offset: number }>(
      `/api/v1/runs/${runId}/fs/certificates?${q.toString()}`
    )
  },

  getTLSSummary: (runId: string) =>
    fetchJSON<TLSSummary>(`/api/v1/runs/${runId}/tls/summary`),

  getTLSObservationsNone: (runId: string, params: {
    q?: string
    version?: string
    cipher?: string
    direction?: string
    attribution?: string
    limit?: number
    offset?: number
  }) => {
    const q = new URLSearchParams()
    q.set('group', 'none')
    if (params.q) q.set('q', params.q)
    if (params.version) q.set('version', params.version)
    if (params.cipher) q.set('cipher', params.cipher)
    if (params.direction) q.set('direction', params.direction)
    if (params.attribution) q.set('attribution', params.attribution)
    if (params.limit !== undefined) q.set('limit', String(params.limit))
    if (params.offset !== undefined) q.set('offset', String(params.offset))
    return fetchJSON<{ items: TLSObservationItem[]; total: number; limit: number; offset: number }>(
      `/api/v1/runs/${runId}/tls/observations?${q.toString()}`
    )
  },

  getTLSObservationsGrouped: (runId: string, params?: {
    q?: string
    version?: string
    cipher?: string
    direction?: string
    attribution?: string
  }) => {
    const q = new URLSearchParams()
    q.set('group', 'endpoint')
    if (params?.q) q.set('q', params.q)
    if (params?.version) q.set('version', params.version)
    if (params?.cipher) q.set('cipher', params.cipher)
    if (params?.direction) q.set('direction', params.direction)
    if (params?.attribution) q.set('attribution', params.attribution)
    return fetchJSON<{ items: TLSEndpointGroupItem[]; total: number }>(
      `/api/v1/runs/${runId}/tls/observations?${q.toString()}`
    )
  },

  getTLSEvidence: (runId: string) =>
    fetchJSON<TLSEvidence>(`/api/v1/runs/${runId}/tls/evidence`),

  getRunGraph: (runId: string) =>
    fetchJSON<GraphData>(`/api/v1/runs/${runId}/graph`),

  getGlobalGraph: () =>
    fetchJSON<GraphData>('/api/v1/graph?scope=global'),

  generateTraffic: (durationSec = 30) =>
    fetchJSON<{ status: string; duration_sec: number; message: string }>('/api/v1/traffic/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ duration_sec: durationSec }),
    }),
}
