import React, { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'
import {
  TLSEndpointGroupItem,
  TLSObservationItem,
} from '../types'
import { AsyncState } from '../components/AsyncState'
import { StatusPill } from '../components/StatusPill'
import { PQCBadge } from '../components/PQCBadge'
import { RelationshipMap } from '../components/RelationshipMap'
import {
  ArrowLeft,
  Activity,
  Network,
  Globe,
  Radio,
  HelpCircle,
  ChevronDown,
  ChevronRight,
  ShieldCheck,
  ShieldAlert,
  Server,
  Cpu,
  Layers,
  Info,
} from 'lucide-react'
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  Legend,
  AreaChart,
  Area,
  PieChart,
  Pie,
  Cell,
} from 'recharts'

export const TLSReportPage: React.FC = () => {
  const { id } = useParams<{ id: string }>()
  const runId = id || ''

  const [expandedEndpoints, setExpandedEndpoints] = useState<Record<string, boolean>>({})
  const [showEvidence, setShowEvidence] = useState(false)

  // 1. Fetch Run details
  const { data: run, isLoading: isRunLoading } = useQuery({
    queryKey: ['run', runId],
    queryFn: () => api.getRun(runId),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'queued' || status === 'running' ? 1000 : false
    },
  })

  // 2. Fetch TLS Summary
  const {
    data: summary,
    isLoading: isSummaryLoading,
    error: summaryError,
    refetch: refetchSummary,
  } = useQuery({
    queryKey: ['tls-summary', runId],
    queryFn: () => api.getTLSSummary(runId),
    enabled: !!runId,
  })

  // 3. Fetch Grouped observations
  const { data: groupedData, isLoading: isGroupedLoading } = useQuery({
    queryKey: ['tls-grouped', runId],
    queryFn: () => api.getTLSObservationsGrouped(runId),
    enabled: !!runId,
  })
  const endpointGroups = groupedData?.items || []

  // 4. Fetch Flat observations (for detail drawer / expansion)
  const { data: flatData } = useQuery({
    queryKey: ['tls-flat', runId],
    queryFn: () => api.getTLSObservationsNone(runId, { limit: 200 }),
    enabled: !!runId,
  })
  const flatObservations = flatData?.items || []

  // 5. Fetch Evidence
  const { data: evidence } = useQuery({
    queryKey: ['tls-evidence', runId],
    queryFn: () => api.getTLSEvidence(runId),
    enabled: !!runId,
  })

  // 6. Fetch Graphs
  const { data: runGraph, isLoading: isRunGraphLoading } = useQuery({
    queryKey: ['run-graph', runId],
    queryFn: () => api.getRunGraph(runId),
    enabled: !!runId,
  })

  const { data: globalGraph } = useQuery({
    queryKey: ['global-graph'],
    queryFn: () => api.getGlobalGraph(),
  })

  const toggleEndpoint = (key: string) => {
    setExpandedEndpoints((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  // Cert posture colors
  const postureColors: Record<string, string> = {
    valid: '#10b981',
    expiring: '#f59e0b',
    expired: '#ef4444',
    unknown: '#64748b',
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-slate-800">
        <div className="flex items-center gap-4">
          <Link
            to="/runs"
            className="p-2 rounded-lg bg-slate-900 border border-slate-800 hover:bg-slate-800 text-slate-400 hover:text-white transition"
          >
            <ArrowLeft className="w-5 h-5" />
          </Link>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-bold text-white tracking-tight">
                TLS Metadata Scan Report
              </h1>
              {run && <StatusPill status={run.status} />}
            </div>
            <div className="flex items-center gap-2 mt-1">
              <span className="text-xs text-slate-400 font-mono">Run ID: {runId}</span>
              {run?.capture_method && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[11px] font-mono bg-sky-950/60 border border-sky-800/80 text-sky-300">
                  <Radio className="w-3 h-3 text-sky-400" />
                  {run.capture_method}
                  {run.capture_note ? ` (${run.capture_note})` : ''}
                </span>
              )}
            </div>
          </div>
        </div>
      </div>

      <AsyncState
        isLoading={isRunLoading || isSummaryLoading}
        error={summaryError as Error}
        onRetry={refetchSummary}
        run={run}
      >
        {summary && (
          <>
            {/* 1. Metric Cards */}
            <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
              <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">
                  Total Events
                </div>
                <div className="text-3xl font-bold text-white mt-1">{summary.events}</div>
                <div className="text-[11px] text-slate-500 mt-0.5">TLS record headers</div>
              </div>

              <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-xs font-medium text-cyan-400 uppercase tracking-wider">
                  Connections
                </div>
                <div className="text-3xl font-bold text-cyan-400 mt-1">{summary.connections}</div>
                <div className="text-[11px] text-slate-500 mt-0.5">Observed flows</div>
              </div>

              <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-xs font-medium text-sky-400 uppercase tracking-wider">
                  Unique Endpoints
                </div>
                <div className="text-3xl font-bold text-sky-400 mt-1">
                  {summary.unique_endpoints}
                </div>
                <div className="text-[11px] text-slate-500 mt-0.5">Across {summary.unique_ports} ports</div>
              </div>

              <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-xs font-medium text-emerald-400 uppercase tracking-wider">
                  Certs Observed
                </div>
                <div className="text-3xl font-bold text-emerald-400 mt-1">
                  {summary.certs_observed}
                </div>
                <div className="text-[11px] text-slate-500 mt-0.5">Observed & probed</div>
              </div>

              <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-xs font-medium text-purple-400 uppercase tracking-wider">
                  Relationships
                </div>
                <div className="text-3xl font-bold text-purple-400 mt-1">
                  {summary.relationships}
                </div>
                <div className="text-[11px] text-slate-500 mt-0.5">Mapped graph edges</div>
              </div>
            </div>

            {/* 2. "How is this total computed?" Expander */}
            <div className="rounded-xl border border-slate-800 bg-slate-900/50 overflow-hidden">
              <button
                onClick={() => setShowEvidence(!showEvidence)}
                className="w-full flex items-center justify-between p-3.5 text-xs font-semibold text-slate-300 hover:text-white hover:bg-slate-800/40 transition cursor-pointer"
              >
                <span className="flex items-center gap-2">
                  <HelpCircle className="w-4 h-4 text-cyan-400" />
                  How is this total computed? (Audit & Mathematical Proof)
                </span>
                {showEvidence ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
              </button>

              {showEvidence && evidence && (
                <div className="p-4 border-t border-slate-800 bg-slate-950/80 space-y-4 text-xs">
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                    <div className="p-3 rounded-lg bg-slate-900 border border-slate-800 space-y-1">
                      <div className="font-semibold text-slate-200">Events Formula</div>
                      <div className="font-mono text-cyan-300 text-[11px]">{evidence.events.formula}</div>
                      <div className="text-slate-400 text-[11px] pt-1">
                        Breakdown: {evidence.events.breakdown.handshake} handshake,{' '}
                        {evidence.events.breakdown.appdata} appdata, {evidence.events.breakdown.alert} alert
                      </div>
                    </div>

                    <div className="p-3 rounded-lg bg-slate-900 border border-slate-800 space-y-1">
                      <div className="font-semibold text-slate-200">Unique Endpoints Formula</div>
                      <div className="font-mono text-cyan-300 text-[11px]">
                        {evidence.unique_endpoints.formula}
                      </div>
                      <div className="text-slate-400 text-[11px] pt-1">
                        Total: {evidence.unique_endpoints.total} distinct (remote_ip, remote_port) pairs
                      </div>
                    </div>

                    <div className="p-3 rounded-lg bg-slate-900 border border-slate-800 space-y-1">
                      <div className="font-semibold text-slate-200">Certs Observed</div>
                      <div className="font-mono text-cyan-300 text-[11px]">
                        COUNT(DISTINCT peer_cert_fp)
                      </div>
                      <div className="text-slate-400 text-[11px] pt-1">
                        Total: {evidence.certs_observed.total} distinct SHA-256 fingerprints
                      </div>
                    </div>
                  </div>

                  {/* Top connections breakdown */}
                  {evidence.events.by_connection && evidence.events.by_connection.length > 0 && (
                    <div className="space-y-1.5 pt-2">
                      <div className="font-semibold text-slate-300 text-[11px] uppercase tracking-wider">
                        Top Connections by Event Count
                      </div>
                      <div className="max-h-40 overflow-y-auto rounded border border-slate-800">
                        <table className="w-full text-left text-[11px]">
                          <thead className="bg-slate-900 text-slate-400 sticky top-0">
                            <tr>
                              <th className="p-2">Remote Endpoint</th>
                              <th className="p-2">SNI</th>
                              <th className="p-2 text-right">Records</th>
                            </tr>
                          </thead>
                          <tbody className="divide-y divide-slate-800 text-slate-300 font-mono">
                            {evidence.events.by_connection.map((c, i) => (
                              <tr key={i}>
                                <td className="p-2">{c.remote}</td>
                                <td className="p-2">{c.sni || '—'}</td>
                                <td className="p-2 text-right text-cyan-400">{c.record_count}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>

            {/* 3. Recharts Visualizations */}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {/* TLS Versions */}
              <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
                <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-3">
                  Protocol Versions
                </h3>
                <div className="h-44 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={summary.versions}>
                      <XAxis dataKey="name" stroke="#64748b" fontSize={11} />
                      <YAxis stroke="#64748b" fontSize={11} allowDecimals={false} />
                      <Tooltip contentStyle={{ backgroundColor: '#0f172a', borderColor: '#334155', borderRadius: 8, fontSize: 12 }} />
                      <Bar dataKey="count" name="Observations" fill="#38bdf8" radius={[4, 4, 0, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </div>

              {/* Activity Timeline */}
              <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
                <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-3">
                  Activity Timeline (5s Buckets)
                </h3>
                <div className="h-44 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <AreaChart
                      data={summary.timeline.map((t) => ({
                        time: new Date(t.ts).toLocaleTimeString([], { minute: '2-digit', second: '2-digit' }),
                        events: t.events,
                      }))}
                    >
                      <XAxis dataKey="time" stroke="#64748b" fontSize={11} />
                      <YAxis stroke="#64748b" fontSize={11} allowDecimals={false} />
                      <Tooltip contentStyle={{ backgroundColor: '#0f172a', borderColor: '#334155', borderRadius: 8, fontSize: 12 }} />
                      <Area
                        type="monotone"
                        dataKey="events"
                        name="Events"
                        stroke="#06b6d4"
                        fill="#06b6d4"
                        fillOpacity={0.25}
                      />
                    </AreaChart>
                  </ResponsiveContainer>
                </div>
              </div>

              {/* Certificate Posture */}
              <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
                <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-3">
                  Certificate Posture
                </h3>
                <div className="h-44 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <Pie
                        data={summary.cert_posture}
                        innerRadius={40}
                        outerRadius={65}
                        paddingAngle={4}
                        dataKey="count"
                        nameKey="name"
                      >
                        {summary.cert_posture.map((entry, idx) => (
                          <Cell
                            key={`cell-${idx}`}
                            fill={postureColors[entry.name.toLowerCase()] || '#64748b'}
                          />
                        ))}
                      </Pie>
                      <Tooltip contentStyle={{ backgroundColor: '#0f172a', borderColor: '#334155', borderRadius: 8, fontSize: 12 }} />
                      <Legend wrapperStyle={{ fontSize: 11 }} />
                    </PieChart>
                  </ResponsiveContainer>
                </div>
              </div>
            </div>

            {/* 4. Relationship Map (React Flow + dagre) */}
            <RelationshipMap
              runId={runId}
              runGraph={runGraph || null}
              globalGraph={globalGraph || null}
              isLoading={isRunGraphLoading}
            />

            {/* 5. Grouped-by-endpoint Table with expandable rows */}
            <div className="rounded-xl border border-slate-800 bg-slate-900/40 overflow-hidden shadow-lg space-y-0">
              <div className="p-4 border-b border-slate-800 bg-slate-950/60 flex items-center justify-between">
                <h2 className="text-sm font-semibold text-slate-200 flex items-center gap-2">
                  <Globe className="w-4 h-4 text-cyan-400" />
                  Observed Network Endpoints & Flows
                </h2>
                <span className="text-xs text-slate-400">
                  {endpointGroups.length} unique remote destinations
                </span>
              </div>

              <div className="overflow-x-auto">
                <table className="w-full text-left border-collapse text-sm">
                  <thead>
                    <tr className="border-b border-slate-800 bg-slate-950/40 text-slate-400 font-medium text-xs uppercase tracking-wider">
                      <th className="py-3 px-4 w-8"></th>
                      <th className="py-3 px-4">Remote Endpoint & SNI</th>
                      <th className="py-3 px-4">Flows / Events</th>
                      <th className="py-3 px-4">TLS Versions</th>
                      <th className="py-3 px-4">Cipher Suites</th>
                      <th className="py-3 px-4">Attributed Processes</th>
                      <th className="py-3 px-4">Directions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-800/60 text-slate-200">
                    {endpointGroups.map((group, idx) => {
                      const epKey = `${group.remote_ip}:${group.remote_port}`
                      const isExpanded = !!expandedEndpoints[epKey]

                      // Find matching connections
                      const matchedConns = flatObservations.filter(
                        (o) => o.remote_ip === group.remote_ip && o.remote_port === group.remote_port
                      )

                      return (
                        <React.Fragment key={idx}>
                          <tr
                            onClick={() => toggleEndpoint(epKey)}
                            className="hover:bg-slate-800/40 cursor-pointer transition"
                          >
                            <td className="py-3 px-4 text-slate-400">
                              {isExpanded ? (
                                <ChevronDown className="w-4 h-4" />
                              ) : (
                                <ChevronRight className="w-4 h-4" />
                              )}
                            </td>
                            <td className="py-3 px-4">
                              <div className="font-mono text-xs font-semibold text-cyan-300">
                                {group.remote_ip}:{group.remote_port}
                              </div>
                              {group.sni && (
                                <div className="text-xs text-slate-400">{group.sni}</div>
                              )}
                            </td>

                            <td className="py-3 px-4 text-xs font-mono">
                              <span className="text-slate-200 font-semibold">{group.connections}</span>
                              <span className="text-slate-500"> / </span>
                              <span className="text-cyan-400">{group.events} events</span>
                            </td>

                            <td className="py-3 px-4 text-xs font-mono text-slate-300">
                              {group.versions.join(', ') || '—'}
                            </td>

                            <td className="py-3 px-4 text-xs font-mono text-slate-400 max-w-[200px] truncate" title={group.ciphers.join(', ')}>
                              {group.ciphers.join(', ') || '—'}
                            </td>

                            <td className="py-3 px-4 text-xs">
                              {group.processes.length > 0 ? (
                                <div className="flex flex-wrap gap-1">
                                  {group.processes.map((p, pi) => (
                                    <span
                                      key={pi}
                                      className="px-2 py-0.5 rounded bg-slate-800 text-slate-300 font-mono text-[11px]"
                                    >
                                      {p}
                                    </span>
                                  ))}
                                </div>
                              ) : (
                                <span className="text-slate-500 text-xs italic">unavailable</span>
                              )}
                            </td>

                            <td className="py-3 px-4 text-xs capitalize text-slate-300">
                              {group.directions.join(', ')}
                            </td>
                          </tr>

                          {/* Expanded Per-Connection Rows */}
                          {isExpanded && (
                            <tr>
                              <td colSpan={7} className="p-0 bg-slate-950/70 border-b border-slate-800">
                                <div className="p-4 pl-12 space-y-3">
                                  <div className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                                    Individual Flow Observations ({matchedConns.length})
                                  </div>
                                  <div className="space-y-2">
                                    {matchedConns.map((conn) => (
                                      <div
                                        key={conn.obs_id}
                                        className="p-3 rounded-lg bg-slate-900 border border-slate-800 text-xs space-y-2"
                                      >
                                        <div className="flex flex-wrap items-center justify-between gap-2">
                                          <div className="flex items-center gap-2">
                                            <span className="font-mono text-cyan-300">
                                              {conn.direction} • {conn.local_ip}:{conn.local_port} →{' '}
                                              {conn.remote_ip}:{conn.remote_port}
                                            </span>
                                            <span className="text-slate-500">•</span>
                                            <span className="text-slate-400">{conn.tls_version || 'Unknown TLS'}</span>
                                          </div>
                                          <div className="flex items-center gap-2">
                                            <PQCBadge pqcClass={conn.pqc_class} />
                                            {conn.peer_cert_source && (
                                              <span
                                                className={`px-2 py-0.5 rounded text-[10px] uppercase font-semibold border ${
                                                  conn.peer_cert_source === 'observed'
                                                    ? 'bg-emerald-950/50 text-emerald-300 border-emerald-800'
                                                    : conn.peer_cert_source === 'probed'
                                                    ? 'bg-sky-950/50 text-sky-300 border-sky-800'
                                                    : 'bg-slate-800 text-slate-400 border-slate-700'
                                                }`}
                                              >
                                                {conn.peer_cert_source}
                                              </span>
                                            )}
                                          </div>
                                        </div>

                                        <div className="grid grid-cols-2 md:grid-cols-4 gap-2 text-slate-400 text-[11px] pt-1 border-t border-slate-800/80">
                                          <div>
                                            Cipher: <span className="font-mono text-slate-200">{conn.cipher_suite || '—'}</span>
                                          </div>
                                          <div>
                                            Process:{' '}
                                            <span className="font-mono text-slate-200">
                                              {conn.exe || conn.comm || conn.attribution || 'unavailable'}
                                              {conn.pid ? ` (${conn.pid})` : ''}
                                            </span>
                                          </div>
                                          <div>
                                            Records:{' '}
                                            <span className="font-mono text-slate-200">
                                              {conn.record_count} (HS: {conn.handshake_count}, App: {conn.appdata_count})
                                            </span>
                                          </div>
                                          <div>
                                            Handshake Seq:{' '}
                                            <span className="font-mono text-slate-200">{conn.handshake_seq || '—'}</span>
                                          </div>
                                        </div>
                                      </div>
                                    ))}
                                  </div>
                                </div>
                              </td>
                            </tr>
                          )}
                        </React.Fragment>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          </>
        )}
      </AsyncState>
    </div>
  )
}
