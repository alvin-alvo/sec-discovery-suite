export type RunType = 'filesystem' | 'tls'
export type RunTrigger = 'manual' | 'scheduled'
export type RunStatus = 'queued' | 'running' | 'completed' | 'partial' | 'failed'
export type PQCClass = 'classical' | 'hybrid' | 'pqc' | 'unknown'

export interface Run {
  run_id: string
  type: RunType
  trigger: RunTrigger
  status: RunStatus
  params?: any
  requested_at: string
  started_at?: string | null
  finished_at?: string | null
  duration_ms?: number | null
  records_total: number
  records_unique: number
  error_count: number
  error_summary?: string | null
  agent_version?: string | null
  capture_method?: string | null
  capture_note?: string | null
  privileged?: boolean | null
  is_sample?: boolean
}

export interface AgentStatus {
  online: boolean
  version?: string
  uid?: number
  privileged?: boolean
  tiers?: string[]
  binary_bytes?: number
  error?: string
}

export interface Schedule {
  id: string
  type: RunType
  interval_seconds: number
  enabled: boolean
  params?: any
  last_run_id?: string | null
  next_run_at?: string | null
}

export interface NameCount {
  name: string
  count: number
}

export interface FSSummary {
  found: number
  unique: number
  duplicates: number
  valid: number
  expiring: number
  expired: number
  parse_failed: number
  access_denied: number
  algorithms: NameCount[]
  key_sizes: NameCount[]
  sig_algos: NameCount[]
  top_issuers: NameCount[]
  locations: NameCount[]
}

export interface FSCertificateItem {
  obs_id: number
  run_id: string
  fingerprint_sha256?: string | null
  path: string
  resolved_path?: string | null
  filename?: string | null
  format?: string | null
  scan_root?: string | null
  chain_position: number
  chain_length?: number | null
  file_mode?: string | null
  file_size?: number | null
  is_symlink: boolean
  contains_private_key: boolean
  parse_status: string
  parse_error?: string | null
  days_to_expiry?: number | null
  validity_status?: string | null
  owner_pid?: number | null
  owner_proc?: string | null
  owner_unit?: string | null
  owner_confidence?: string | null
  discovered_at?: string | null
  serial?: string | null
  subject_dn?: string | null
  subject_cn?: string | null
  issuer_dn?: string | null
  issuer_cn?: string | null
  san?: string[] | null
  not_before?: string | null
  not_after?: string | null
  pubkey_algo?: string | null
  pubkey_bits?: number | null
  curve?: string | null
  sig_algo?: string | null
  is_ca?: boolean | null
  is_self_signed?: boolean | null
  pqc_class?: PQCClass | null
}

export interface FSFacets {
  validity_status: string[]
  issuers: string[]
  algorithms: string[]
  formats: string[]
  parse_status: string[]
}

export interface TLSSummary {
  events: number
  connections: number
  unique_endpoints: number
  unique_ports: number
  certs_observed: number
  relationships: number
  versions: NameCount[]
  ciphers: NameCount[]
  kex_groups: NameCount[]
  directions: NameCount[]
  peer_cert_sources: NameCount[]
  attribution: NameCount[]
  top_endpoints: NameCount[]
  cert_posture: NameCount[]
  timeline: { ts: string; events: number }[]
}

export interface TLSObservationItem {
  obs_id: number
  run_id: string
  conn_key: string
  first_ts?: string | null
  last_ts?: string | null
  direction?: string | null
  transport: string
  local_ip?: string | null
  local_port?: number | null
  remote_ip?: string | null
  remote_port?: number | null
  sni?: string | null
  tls_version?: string | null
  cipher_suite?: string | null
  kex_group?: string | null
  pqc_class?: PQCClass | null
  outcome?: string | null
  peer_cert_fp?: string | null
  peer_cert_source?: string | null
  pid?: number | null
  exe?: string | null
  comm?: string | null
  unit?: string | null
  attribution?: string | null
  record_count: number
  handshake_count: number
  appdata_count: number
  alert_count: number
  handshake_seq?: string | null
  peer_subject_cn?: string | null
  peer_issuer_cn?: string | null
  peer_not_after?: string | null
  peer_pubkey_algo?: string | null
  peer_pubkey_bits?: number | null
  peer_validity?: string | null
}

export interface TLSEndpointGroupItem {
  remote_ip: string
  remote_port: number
  sni?: string | null
  connections: number
  events: number
  versions: string[]
  ciphers: string[]
  processes: string[]
  directions: string[]
}

export interface TLSEvidence {
  events: {
    total: number
    formula: string
    breakdown: {
      handshake: number
      appdata: number
      alert: number
      other: number
    }
    by_connection: {
      conn_key: string
      remote: string
      sni?: string | null
      record_count: number
    }[]
  }
  unique_endpoints: {
    total: number
    formula: string
    items: string[]
  }
  certs_observed: {
    total: number
    items: {
      fingerprint_sha256: string
      source: string
    }[]
  }
}

export interface GraphNode {
  id: string
  type: 'host' | 'process' | 'endpoint' | 'certificate'
  label?: string | null
  props?: any
  first_seen?: string | null
  last_seen?: string | null
}

export interface GraphEdge {
  id: string
  src: string
  dst: string
  type: 'RUNS' | 'CONNECTS_TO' | 'PRESENTS' | 'USES' | 'STORES'
  weight: number
  first_seen?: string | null
  last_seen?: string | null
  confidence?: string | null
}

export interface GraphData {
  nodes: GraphNode[]
  edges: GraphEdge[]
}
