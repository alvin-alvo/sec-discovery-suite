import React, { useState, useRef, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery, useInfiniteQuery } from '@tanstack/react-query'
import { api } from '../api'
import { FSCertificateItem } from '../types'
import { AsyncState } from '../components/AsyncState'
import { StatusPill } from '../components/StatusPill'
import { PQCBadge } from '../components/PQCBadge'
import {
  ArrowLeft,
  Key,
  Copy,
  Check,
  Search,
  Filter,
  ShieldCheck,
  ShieldAlert,
  ShieldX,
  FileCode2,
  FolderLock,
  ChevronRight,
  ChevronLeft,
  ChevronDown,
  X,
  Layers,
  FileCheck,
} from 'lucide-react'
import {
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  Legend,
} from 'recharts'

export const FSReportPage: React.FC = () => {
  const { id } = useParams<{ id: string }>()
  const runId = id || ''

  // Search & Filter state
  const [search, setSearch] = useState('')
  const [filterStatus, setFilterStatus] = useState('')
  const [filterIssuer, setFilterIssuer] = useState('')
  const [filterAlgo, setFilterAlgo] = useState('')
  const [filterFormat, setFilterFormat] = useState('')
  const [filterParseStatus, setFilterParseStatus] = useState('')

  // Selected cert drawer
  const [selectedCert, setSelectedCert] = useState<FSCertificateItem | null>(null)
  const [copiedFP, setCopiedFP] = useState(false)

  // 1. Fetch Run details
  const { data: run, isLoading: isRunLoading } = useQuery({
    queryKey: ['run', runId],
    queryFn: () => api.getRun(runId),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'queued' || status === 'running' ? 1000 : false
    },
  })

  const isRunning = run?.status === 'queued' || run?.status === 'running'

  // 2. Fetch FS Summary (polls every 1s when running)
  const { data: summary, isLoading: isSummaryLoading, error: summaryError, refetch: refetchSummary } = useQuery({
    queryKey: ['fs-summary', runId],
    queryFn: () => api.getFSSummary(runId),
    enabled: !!runId,
    refetchInterval: isRunning ? 1000 : false,
  })

  // 3. Fetch Facets for filters (polls every 1s when running)
  const { data: facets } = useQuery({
    queryKey: ['fs-facets', runId],
    queryFn: () => api.getFSFacets(runId),
    enabled: !!runId,
    refetchInterval: isRunning ? 1000 : false,
  })

  // Infinite query for dynamic loading of certificates
  const {
    data: certsInfiniteData,
    isLoading: isCertsLoading,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    refetch: refetchCerts,
  } = useInfiniteQuery({
    queryKey: ['fs-certs-infinite', runId, search, filterStatus, filterIssuer, filterAlgo, filterFormat, filterParseStatus],
    queryFn: ({ pageParam = 0 }) =>
      api.getFSCertificates(runId, {
        q: search || undefined,
        status: filterStatus || undefined,
        issuer: filterIssuer || undefined,
        algo: filterAlgo || undefined,
        format: filterFormat || undefined,
        parse_status: filterParseStatus || undefined,
        limit: 50,
        offset: pageParam,
      }),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => {
      const loadedCount = allPages.reduce((acc, p) => acc + (p.items?.length || 0), 0)
      return loadedCount < (lastPage?.total || 0) ? loadedCount : undefined
    },
    enabled: !!runId,
    refetchInterval: isRunning ? 1000 : false,
  })

  const certs = React.useMemo(() => {
    return certsInfiniteData?.pages.flatMap((p) => p.items || []) || []
  }, [certsInfiniteData])

  const totalCerts = certsInfiniteData?.pages[0]?.total || 0

  // Sentinel ref for dynamic infinite scroll
  const sentinelRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!sentinelRef.current || !hasNextPage || isFetchingNextPage) return
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
          fetchNextPage()
        }
      },
      { threshold: 0.1, rootMargin: '250px' }
    )
    observer.observe(sentinelRef.current)
    return () => observer.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    setCopiedFP(true)
    setTimeout(() => setCopiedFP(false), 2000)
  }

  // Validity Donut chart data
  const validityDonutData = React.useMemo(() => {
    if (!summary) return []
    return [
      { name: 'Valid', value: summary.valid, color: '#10b981' },
      { name: 'Expiring (<=30d)', value: summary.expiring, color: '#f59e0b' },
      { name: 'Expired', value: summary.expired, color: '#ef4444' },
      { name: 'Parse Failed', value: summary.parse_failed, color: '#8b5cf6' },
      { name: 'Access Denied', value: summary.access_denied, color: '#64748b' },
    ].filter((d) => d.value > 0)
  }, [summary])

  return (
    <div className="space-y-6">
      {/* Back button and page title */}
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
                Filesystem Discovery Report
              </h1>
              {run && <StatusPill status={run.status} />}
            </div>
            <p className="text-xs text-slate-400 font-mono mt-0.5">Run ID: {runId}</p>
          </div>
        </div>
      </div>

      {/* Live Scan Active Banner */}
      {run?.status === 'running' && (
        <div className="p-4 rounded-xl bg-blue-950/40 border border-blue-800/80 flex items-center justify-between gap-4 animate-in fade-in duration-300">
          <div className="flex items-center gap-3">
            <span className="relative flex h-3.5 w-3.5">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-blue-400 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-3.5 w-3.5 bg-blue-500"></span>
            </span>
            <div>
              <div className="text-sm font-semibold text-blue-300 flex items-center gap-2">
                Live Filesystem Discovery in Progress
                <span className="text-[11px] px-2 py-0.5 rounded bg-blue-900/80 text-blue-200 font-mono">
                  streaming live
                </span>
              </div>
              <div className="text-xs text-slate-400 mt-0.5">
                Scanning certificate stores and deduplicating fingerprints ({summary?.found || 0} certificates found so far)
              </div>
            </div>
          </div>
        </div>
      )}

      <AsyncState
        isLoading={isRunLoading || isSummaryLoading}
        error={summaryError as Error}
        onRetry={refetchSummary}
        run={run}
      >
        {summary && (
          <>
            {/* 1. Metric Cards */}
            <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-8 gap-3">
              <div className="p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-[11px] font-medium text-slate-400 uppercase tracking-wider">Found</div>
                <div className="text-2xl font-bold text-white mt-1">{summary.found}</div>
              </div>
              <div className="p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-[11px] font-medium text-slate-400 uppercase tracking-wider">Unique</div>
                <div className="text-2xl font-bold text-cyan-400 mt-1">{summary.unique}</div>
              </div>
              <div className="p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-[11px] font-medium text-slate-400 uppercase tracking-wider">Duplicates</div>
                <div className="text-2xl font-bold text-slate-300 mt-1">{summary.duplicates}</div>
              </div>
              <div className="p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-[11px] font-medium text-emerald-400 uppercase tracking-wider">Valid</div>
                <div className="text-2xl font-bold text-emerald-400 mt-1">{summary.valid}</div>
              </div>
              <div className="p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-[11px] font-medium text-amber-400 uppercase tracking-wider">Expiring</div>
                <div className="text-2xl font-bold text-amber-400 mt-1">{summary.expiring}</div>
              </div>
              <div className="p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-[11px] font-medium text-rose-400 uppercase tracking-wider">Expired</div>
                <div className="text-2xl font-bold text-rose-400 mt-1">{summary.expired}</div>
              </div>
              <div className="p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-[11px] font-medium text-purple-400 uppercase tracking-wider">Parse Failed</div>
                <div className="text-2xl font-bold text-purple-400 mt-1">{summary.parse_failed}</div>
              </div>
              <div className="p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
                <div className="text-[11px] font-medium text-slate-400 uppercase tracking-wider">Access Denied</div>
                <div className="text-2xl font-bold text-slate-400 mt-1">{summary.access_denied}</div>
              </div>
            </div>

            {/* 2. Recharts Visualizations */}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {/* Validity Donut */}
              <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
                <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-3">
                  Certificate Validity Posture
                </h3>
                <div className="h-48 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <Pie
                        data={validityDonutData}
                        innerRadius={45}
                        outerRadius={70}
                        paddingAngle={4}
                        dataKey="value"
                      >
                        {validityDonutData.map((entry, idx) => (
                          <Cell key={`cell-${idx}`} fill={entry.color} />
                        ))}
                      </Pie>
                      <Tooltip contentStyle={{ backgroundColor: '#0f172a', borderColor: '#334155', borderRadius: 8, fontSize: 12 }} />
                      <Legend wrapperStyle={{ fontSize: 11 }} />
                    </PieChart>
                  </ResponsiveContainer>
                </div>
              </div>

              {/* Algorithms and Key Sizes */}
              <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
                <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-3">
                  Key Sizes & Algorithms
                </h3>
                <div className="h-48 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={summary.key_sizes}>
                      <XAxis dataKey="name" stroke="#64748b" fontSize={11} />
                      <YAxis stroke="#64748b" fontSize={11} allowDecimals={false} />
                      <Tooltip contentStyle={{ backgroundColor: '#0f172a', borderColor: '#334155', borderRadius: 8, fontSize: 12 }} />
                      <Bar dataKey="count" name="Count" fill="#38bdf8" radius={[4, 4, 0, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </div>

              {/* Top Issuers */}
              <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
                <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-3">
                  Top Certificate Issuers
                </h3>
                <div className="h-48 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart layout="vertical" data={summary.top_issuers.slice(0, 5)}>
                      <XAxis type="number" stroke="#64748b" fontSize={11} allowDecimals={false} />
                      <YAxis dataKey="name" type="category" stroke="#64748b" fontSize={11} width={100} />
                      <Tooltip contentStyle={{ backgroundColor: '#0f172a', borderColor: '#334155', borderRadius: 8, fontSize: 12 }} />
                      <Bar dataKey="count" name="Count" fill="#a855f7" radius={[0, 4, 4, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </div>
            </div>

            {/* 3. Search & Filter Bar */}
            <div className="p-4 rounded-xl bg-slate-900/40 border border-slate-800 space-y-3">
              <div className="flex flex-col md:flex-row gap-3">
                <div className="relative flex-1">
                  <Search className="w-4 h-4 text-slate-400 absolute left-3 top-3" />
                  <input
                    type="text"
                    value={search}
                    onChange={(e) => {
                      setSearch(e.target.value)
                    }}
                    placeholder="Search by subject, issuer, SAN or path..."
                    className="w-full bg-slate-950/80 border border-slate-700/80 rounded-lg pl-9 pr-4 py-2 text-sm text-slate-200 placeholder-slate-500 focus:outline-none focus:border-cyan-500"
                  />
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  <select
                    value={filterStatus}
                    onChange={(e) => {
                      setFilterStatus(e.target.value)
                    }}
                    className="bg-slate-950/80 border border-slate-700/80 rounded-lg px-3 py-2 text-xs text-slate-300 focus:outline-none focus:border-cyan-500"
                  >
                    <option value="">All Validity</option>
                    {facets?.validity_status?.map((s) => (
                      <option key={s} value={s}>
                        {s}
                      </option>
                    ))}
                  </select>

                  <select
                    value={filterAlgo}
                    onChange={(e) => {
                      setFilterAlgo(e.target.value)
                    }}
                    className="bg-slate-950/80 border border-slate-700/80 rounded-lg px-3 py-2 text-xs text-slate-300 focus:outline-none focus:border-cyan-500"
                  >
                    <option value="">All Algorithms</option>
                    {facets?.algorithms?.map((a) => (
                      <option key={a} value={a}>
                        {a}
                      </option>
                    ))}
                  </select>

                  <select
                    value={filterFormat}
                    onChange={(e) => {
                      setFilterFormat(e.target.value)
                    }}
                    className="bg-slate-950/80 border border-slate-700/80 rounded-lg px-3 py-2 text-xs text-slate-300 focus:outline-none focus:border-cyan-500"
                  >
                    <option value="">All Formats</option>
                    {facets?.formats?.map((f) => (
                      <option key={f} value={f}>
                        {f}
                      </option>
                    ))}
                  </select>

                  <select
                    value={filterParseStatus}
                    onChange={(e) => {
                      setFilterParseStatus(e.target.value)
                    }}
                    className="bg-slate-950/80 border border-slate-700/80 rounded-lg px-3 py-2 text-xs text-slate-300 focus:outline-none focus:border-cyan-500"
                  >
                    <option value="">All Parse Statuses</option>
                    {facets?.parse_status?.map((ps) => (
                      <option key={ps} value={ps}>
                        {ps}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
            </div>

            {/* 4. Certificates Table */}
            <div className="rounded-xl border border-slate-800 bg-slate-900/40 overflow-hidden shadow-lg">
              <div className="overflow-x-auto">
                <table className="w-full text-left border-collapse text-sm">
                  <thead>
                    <tr className="border-b border-slate-800 bg-slate-950/60 text-slate-400 font-medium text-xs uppercase tracking-wider">
                      <th className="py-3 px-4">Subject / Path</th>
                      <th className="py-3 px-4">Issuer</th>
                      <th className="py-3 px-4">Validity</th>
                      <th className="py-3 px-4">Algorithm & Bits</th>
                      <th className="py-3 px-4">Posture</th>
                      <th className="py-3 px-4">Fingerprint</th>
                      <th className="py-3 px-4">Flags</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-800/60 text-slate-200">
                    {certs.map((c) => {
                      const isExpired = c.validity_status === 'expired'
                      const isExpiring = c.validity_status === 'expiring'
                      return (
                        <tr
                          key={c.obs_id}
                          onClick={() => setSelectedCert(c)}
                          className="hover:bg-slate-800/40 cursor-pointer transition"
                        >
                          <td className="py-3 px-4 max-w-xs">
                            <div className="font-semibold text-slate-200 truncate">
                              {c.subject_cn || c.filename || 'Unknown'}
                            </div>
                            <div className="text-[11px] text-slate-400 font-mono truncate" title={c.path}>
                              {c.path}
                            </div>
                          </td>

                          <td className="py-3 px-4 text-xs text-slate-300 max-w-[180px] truncate" title={c.issuer_cn || c.issuer_dn || '—'}>
                            {c.issuer_cn || c.issuer_dn || '—'}
                          </td>

                          <td className="py-3 px-4">
                            {c.parse_status === 'ok' ? (
                              <span
                                className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
                                  isExpired
                                    ? 'bg-rose-950/40 text-rose-300 border border-rose-800/60'
                                    : isExpiring
                                    ? 'bg-amber-950/40 text-amber-300 border border-amber-800/60'
                                    : 'bg-emerald-950/40 text-emerald-300 border border-emerald-800/60'
                                }`}
                              >
                                {c.validity_status}
                              </span>
                            ) : (
                              <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-950/40 text-purple-300 border border-purple-800/60">
                                {c.parse_status}
                              </span>
                            )}
                          </td>

                          <td className="py-3 px-4 text-xs font-mono text-slate-300">
                            {c.pubkey_algo ? `${c.pubkey_algo} ${c.pubkey_bits ? `(${c.pubkey_bits})` : ''}` : '—'}
                          </td>

                          <td className="py-3 px-4">
                            <PQCBadge pqcClass={c.pqc_class} />
                          </td>

                          <td className="py-3 px-4 font-mono text-xs text-slate-400">
                            {c.fingerprint_sha256 ? (
                              <span title={c.fingerprint_sha256} className="text-cyan-400">
                                {c.fingerprint_sha256.slice(0, 12)}…
                              </span>
                            ) : (
                              '—'
                            )}
                          </td>

                          <td className="py-3 px-4">
                            <div className="flex flex-wrap items-center gap-1.5">
                              {c.contains_private_key && (
                                <span
                                  className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-rose-900/50 text-rose-300 border border-rose-700/60"
                                  title="Private key block present"
                                >
                                  PRIVATE KEY
                                </span>
                              )}
                              {c.is_symlink && (
                                <span
                                  className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-sky-900/50 text-sky-300 border border-sky-700/60"
                                  title={`Symlink -> ${c.resolved_path}`}
                                >
                                  SYMLINK
                                </span>
                              )}
                              {c.file_mode && c.file_mode.endsWith('777') && (
                                <span
                                  className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-amber-900/50 text-amber-300 border border-amber-700/60"
                                  title="World readable/writable file mode"
                                >
                                  WORLD-READ
                                </span>
                              )}
                            </div>
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>

              {/* Dynamic scroll sentinel */}
              <div ref={sentinelRef} className="h-2 w-full" />

              {/* Dynamic loading footer */}
              <div className="flex flex-col sm:flex-row items-center justify-between gap-3 px-4 py-3 border-t border-slate-800 bg-slate-950/60 text-xs text-slate-400">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-slate-300">
                    Showing {certs.length} of {totalCerts} findings
                  </span>
                  {certs.length < totalCerts && (
                    <span className="text-[11px] text-cyan-400 bg-cyan-950/40 px-2 py-0.5 rounded border border-cyan-800/50">
                      Dynamically loading on scroll
                    </span>
                  )}
                  {certs.length >= totalCerts && totalCerts > 0 && (
                    <span className="text-[11px] text-emerald-400 bg-emerald-950/40 px-2 py-0.5 rounded border border-emerald-800/50">
                      All {totalCerts} items loaded
                    </span>
                  )}
                </div>

                <div className="flex items-center gap-2">
                  {hasNextPage && (
                    <button
                      onClick={() => fetchNextPage()}
                      disabled={isFetchingNextPage}
                      className="px-3 py-1.5 rounded-lg bg-cyan-600/20 hover:bg-cyan-600/30 border border-cyan-500/30 text-cyan-300 font-medium transition disabled:opacity-50 flex items-center gap-1.5 cursor-pointer"
                    >
                      {isFetchingNextPage ? (
                        <>
                          <span className="w-3.5 h-3.5 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin" />
                          Loading more...
                        </>
                      ) : (
                        <>
                          <span>Load More (+50)</span>
                          <ChevronDown className="w-3.5 h-3.5" />
                        </>
                      )}
                    </button>
                  )}
                </div>
              </div>
            </div>

            {/* 5. Row Detail Drawer */}
            {selectedCert && (
              <div className="fixed inset-y-0 right-0 w-full max-w-lg bg-slate-950 border-l border-slate-800 shadow-2xl z-50 p-6 overflow-y-auto space-y-6">
                <div className="flex items-center justify-between pb-4 border-b border-slate-800">
                  <div className="flex items-center gap-2 text-white font-semibold text-lg">
                    <FileCheck className="w-5 h-5 text-cyan-400" />
                    Certificate Details
                  </div>
                  <button
                    onClick={() => setSelectedCert(null)}
                    className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800"
                  >
                    <X className="w-5 h-5" />
                  </button>
                </div>

                {/* Fingerprint block */}
                {selectedCert.fingerprint_sha256 && (
                  <div className="space-y-1.5">
                    <label className="text-xs uppercase font-semibold text-slate-400 tracking-wider">
                      SHA-256 Fingerprint
                    </label>
                    <div className="flex items-center justify-between p-2.5 rounded-lg bg-slate-900 border border-slate-800 text-xs font-mono text-cyan-300">
                      <span className="break-all">{selectedCert.fingerprint_sha256}</span>
                      <button
                        onClick={() => copyToClipboard(selectedCert.fingerprint_sha256!)}
                        className="ml-2 p-1 text-slate-400 hover:text-white shrink-0"
                        title="Copy fingerprint"
                      >
                        {copiedFP ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
                      </button>
                    </div>
                  </div>
                )}

                {/* Properties */}
                <div className="grid grid-cols-2 gap-4 text-xs">
                  <div>
                    <span className="text-slate-400 block">Subject CN</span>
                    <span className="font-medium text-slate-200">{selectedCert.subject_cn || '—'}</span>
                  </div>
                  <div>
                    <span className="text-slate-400 block">Issuer CN</span>
                    <span className="font-medium text-slate-200">{selectedCert.issuer_cn || '—'}</span>
                  </div>
                  <div>
                    <span className="text-slate-400 block">Chain Position / Length</span>
                    <span className="font-medium text-slate-200">
                      {selectedCert.chain_position} of {selectedCert.chain_length || 1}
                    </span>
                  </div>
                  <div>
                    <span className="text-slate-400 block">File Mode & Size</span>
                    <span className="font-medium text-slate-200">
                      {selectedCert.file_mode || '—'} ({selectedCert.file_size ? `${selectedCert.file_size} B` : '—'})
                    </span>
                  </div>
                  <div>
                    <span className="text-slate-400 block">Not Before</span>
                    <span className="font-medium text-slate-200">{selectedCert.not_before || '—'}</span>
                  </div>
                  <div>
                    <span className="text-slate-400 block">Not After</span>
                    <span className="font-medium text-slate-200">{selectedCert.not_after || '—'}</span>
                  </div>
                </div>

                {/* SAN */}
                {selectedCert.san && selectedCert.san.length > 0 && (
                  <div className="space-y-1.5">
                    <label className="text-xs uppercase font-semibold text-slate-400 tracking-wider">
                      Subject Alternative Names (SAN)
                    </label>
                    <div className="flex flex-wrap gap-1.5">
                      {selectedCert.san.map((s, idx) => (
                        <span
                          key={idx}
                          className="px-2 py-0.5 rounded bg-slate-900 border border-slate-800 text-xs font-mono text-slate-300"
                        >
                          {s}
                        </span>
                      ))}
                    </div>
                  </div>
                )}

                {/* Owner process info */}
                {selectedCert.owner_proc && (
                  <div className="p-3.5 rounded-lg bg-slate-900/80 border border-slate-800 space-y-1 text-xs">
                    <div className="font-semibold text-slate-300 flex items-center justify-between">
                      <span>Owner Process</span>
                      <span className="text-cyan-400 capitalize">{selectedCert.owner_confidence} confidence</span>
                    </div>
                    <div className="text-slate-400 font-mono truncate">{selectedCert.owner_proc}</div>
                    {selectedCert.owner_unit && (
                      <div className="text-slate-500 text-[11px]">Systemd: {selectedCert.owner_unit}</div>
                    )}
                  </div>
                )}
              </div>
            )}
          </>
        )}
      </AsyncState>
    </div>
  )
}
