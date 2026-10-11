import React, { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { Run, Schedule } from '../types'
import { StatusPill } from '../components/StatusPill'
import { AsyncState } from '../components/AsyncState'
import {
  HardDrive,
  Network,
  Clock,
  Play,
  Settings2,
  Calendar,
  AlertTriangle,
  CheckCircle2,
} from 'lucide-react'
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  Legend,
  LineChart,
  Line,
} from 'recharts'

export const RunsPage: React.FC = () => {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [showScheduleModal, setShowScheduleModal] = useState(false)
  const [rootsInput, setRootsInput] = useState('')
  const [durationInput, setDurationInput] = useState(30)
  const [isStartingFS, setIsStartingFS] = useState(false)
  const [isStartingTLS, setIsStartingTLS] = useState(false)

  // 1. Fetch runs, polling while any are queued or running
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['runs'],
    queryFn: () => api.getRuns({ limit: 50 }),
    refetchInterval: (query) => {
      const runs = query.state.data?.runs || []
      const hasActive = runs.some((r) => r.status === 'queued' || r.status === 'running')
      return hasActive ? 1000 : 5000
    },
  })

  const runs = data?.runs || []

  // 2. Schedules
  const { data: schedData, refetch: refetchSchedules } = useQuery({
    queryKey: ['schedules'],
    queryFn: api.getSchedules,
  })
  const schedules = schedData?.schedules || []

  const updateScheduleMutation = useMutation({
    mutationFn: (sched: Schedule) => api.updateSchedule(sched),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['schedules'] })
      refetchSchedules()
    },
  })

  // Start run mutations
  const handleStartFS = async () => {
    try {
      setIsStartingFS(true)
      const roots = rootsInput.trim()
        ? rootsInput.split(',').map((s) => s.trim()).filter(Boolean)
        : null
      const res = await api.createRun('filesystem', roots ? { roots } : undefined)
      await queryClient.invalidateQueries({ queryKey: ['runs'] })
      if (res.run_id) {
        navigate(`/runs/${res.run_id}/fs`)
      }
    } catch (e: any) {
      alert('Failed to start filesystem scan: ' + e.message)
    } finally {
      setIsStartingFS(false)
    }
  }

  const handleStartTLS = async () => {
    try {
      setIsStartingTLS(true)
      const dur = Number(durationInput) || 30
      // Automatically send live TLS traffic so connections appear immediately
      api.generateTraffic(dur + 5).catch(() => {})
      const res = await api.createRun('tls', { duration_sec: dur })
      await queryClient.invalidateQueries({ queryKey: ['runs'] })
      if (res.run_id) {
        navigate(`/runs/${res.run_id}/tls`)
      }
    } catch (e: any) {
      alert('Failed to start TLS scan: ' + e.message)
    } finally {
      setIsStartingTLS(false)
    }
  }

  // Format charts data
  const chartDataRunsByStatus = React.useMemo(() => {
    const fsCounts: Record<string, number> = { completed: 0, partial: 0, failed: 0 }
    const tlsCounts: Record<string, number> = { completed: 0, partial: 0, failed: 0 }

    for (const r of runs) {
      if (r.type === 'filesystem') {
        fsCounts[r.status] = (fsCounts[r.status] || 0) + 1
      } else if (r.type === 'tls') {
        tlsCounts[r.status] = (tlsCounts[r.status] || 0) + 1
      }
    }

    return [
      {
        type: 'Filesystem',
        completed: fsCounts.completed,
        partial: fsCounts.partial,
        failed: fsCounts.failed,
      },
      {
        type: 'TLS Metadata',
        completed: tlsCounts.completed,
        partial: tlsCounts.partial,
        failed: tlsCounts.failed,
      },
    ]
  }, [runs])

  const chartDataDuration = React.useMemo(() => {
    return runs
      .slice(0, 15)
      .reverse()
      .map((r) => ({
        id: r.run_id.slice(-6),
        type: r.type,
        duration_s: r.duration_ms ? Number((r.duration_ms / 1000).toFixed(1)) : 0,
      }))
  }, [runs])

  const formatDuration = (ms?: number | null) => {
    if (!ms && ms !== 0) return '—'
    if (ms < 1000) return `${ms}ms`
    const sec = ms / 1000
    if (sec < 60) return `${sec.toFixed(1)}s`
    const min = Math.floor(sec / 60)
    const remSec = Math.round(sec % 60)
    return `${min}m ${remSec}s`
  }

  const formatTimestamp = (ts?: string | null) => {
    if (!ts) return '—'
    const d = new Date(ts)
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  }

  return (
    <div className="space-y-6">
      {/* Header with trigger buttons and schedule button */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-slate-800">
        <div>
          <h1 className="text-2xl font-bold text-white tracking-tight">Discovery Runs</h1>
          <p className="text-sm text-slate-400 mt-0.5">
            Manage and inspect cryptographic scans across host filesystems and TLS networks.
          </p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={() => setShowScheduleModal(!showScheduleModal)}
            className="inline-flex items-center gap-2 px-3.5 py-2 rounded-lg bg-slate-800/80 hover:bg-slate-700/80 border border-slate-700 text-sm font-medium text-slate-200 transition cursor-pointer"
          >
            <Calendar className="w-4 h-4 text-cyan-400" />
            <span>Schedules</span>
          </button>

          <button
            onClick={handleStartFS}
            disabled={isStartingFS}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-gradient-to-r from-sky-600 to-cyan-600 hover:from-sky-500 hover:to-cyan-500 text-white text-sm font-semibold shadow-md shadow-sky-900/30 transition cursor-pointer disabled:opacity-50"
          >
            <HardDrive className="w-4 h-4" />
            <span>{isStartingFS ? 'Starting...' : 'Run Filesystem Scan'}</span>
          </button>

          <button
            onClick={handleStartTLS}
            disabled={isStartingTLS}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-gradient-to-r from-indigo-600 to-purple-600 hover:from-indigo-500 hover:to-purple-500 text-white text-sm font-semibold shadow-md shadow-indigo-900/30 transition cursor-pointer disabled:opacity-50"
          >
            <Network className="w-4 h-4" />
            <span>{isStartingTLS ? 'Starting...' : 'Run TLS Scan'}</span>
          </button>
        </div>
      </div>

      {/* Schedule Configuration Drawer / Panel */}
      {showScheduleModal && (
        <div className="p-5 rounded-xl bg-slate-900/90 border border-slate-700/80 shadow-xl space-y-4">
          <div className="flex items-center justify-between pb-3 border-b border-slate-800">
            <div className="flex items-center gap-2 text-white font-semibold text-base">
              <Settings2 className="w-5 h-5 text-cyan-400" />
              Automated Discovery Schedules
            </div>
            <button
              onClick={() => setShowScheduleModal(false)}
              className="text-xs text-slate-400 hover:text-white"
            >
              Close
            </button>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {schedules.map((sched) => (
              <div
                key={sched.id}
                className="p-4 rounded-lg bg-slate-950/60 border border-slate-800 space-y-3"
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2 font-medium text-slate-200 capitalize">
                    {sched.type === 'filesystem' ? (
                      <HardDrive className="w-4 h-4 text-sky-400" />
                    ) : (
                      <Network className="w-4 h-4 text-purple-400" />
                    )}
                    {sched.type} Scan Schedule
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      checked={sched.enabled}
                      onChange={(e) => {
                        updateScheduleMutation.mutate({
                          ...sched,
                          enabled: e.target.checked,
                        })
                      }}
                      className="sr-only peer"
                    />
                    <div className="w-9 h-5 bg-slate-800 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-cyan-600"></div>
                  </label>
                </div>

                <div className="flex items-center gap-3">
                  <span className="text-xs text-slate-400">Interval:</span>
                  <select
                    value={sched.interval_seconds}
                    onChange={(e) => {
                      updateScheduleMutation.mutate({
                        ...sched,
                        interval_seconds: Number(e.target.value),
                      })
                    }}
                    className="bg-slate-900 border border-slate-700 text-slate-200 text-xs rounded px-2.5 py-1 focus:ring-1 focus:ring-cyan-500"
                  >
                    <option value={30}>30 seconds (demo)</option>
                    <option value={60}>1 minute</option>
                    <option value={300}>5 minutes</option>
                    <option value={900}>15 minutes</option>
                    <option value={3600}>1 hour</option>
                    <option value={86400}>24 hours</option>
                  </select>
                </div>

                {sched.next_run_at && (
                  <div className="text-[11px] text-slate-500 flex items-center gap-1.5">
                    <Clock className="w-3.5 h-3.5" />
                    Next scheduled run: {new Date(sched.next_run_at).toLocaleTimeString()}
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Metrics & Visualizations (Recharts) */}
      {runs.length > 0 && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-3">
              Runs by Type and Status
            </h3>
            <div className="h-44 w-full">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={chartDataRunsByStatus}>
                  <XAxis dataKey="type" stroke="#64748b" fontSize={12} />
                  <YAxis stroke="#64748b" fontSize={12} allowDecimals={false} />
                  <Tooltip
                    contentStyle={{ backgroundColor: '#0f172a', borderColor: '#334155', borderRadius: 8, fontSize: 12 }}
                  />
                  <Legend wrapperStyle={{ fontSize: 12, paddingTop: 6 }} />
                  <Bar dataKey="completed" name="Completed" fill="#10b981" stackId="a" />
                  <Bar dataKey="partial" name="Partial" fill="#f59e0b" stackId="a" />
                  <Bar dataKey="failed" name="Failed" fill="#ef4444" stackId="a" />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>

          <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-3">
              Recent Scan Duration (Seconds)
            </h3>
            <div className="h-44 w-full">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={chartDataDuration}>
                  <XAxis dataKey="id" stroke="#64748b" fontSize={12} />
                  <YAxis stroke="#64748b" fontSize={12} />
                  <Tooltip
                    contentStyle={{ backgroundColor: '#0f172a', borderColor: '#334155', borderRadius: 8, fontSize: 12 }}
                  />
                  <Line
                    type="monotone"
                    dataKey="duration_s"
                    name="Duration (s)"
                    stroke="#38bdf8"
                    strokeWidth={2}
                    dot={{ fill: '#0284c7', r: 4 }}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>
      )}

      {/* Main Runs Table */}
      <AsyncState
        isLoading={isLoading}
        error={error as Error}
        onRetry={refetch}
        isEmpty={runs.length === 0}
        emptyMessage="No discovery runs have been triggered yet. Click 'Run Filesystem Scan' or 'Run TLS Scan' to begin."
      >
        <div className="rounded-xl border border-slate-800 bg-slate-900/40 overflow-hidden shadow-lg">
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse text-sm">
              <thead>
                <tr className="border-b border-slate-800 bg-slate-950/60 text-slate-400 font-medium text-xs uppercase tracking-wider">
                  <th className="py-3 px-4">Run ID</th>
                  <th className="py-3 px-4">Type</th>
                  <th className="py-3 px-4">Trigger</th>
                  <th className="py-3 px-4">Status</th>
                  <th className="py-3 px-4">Started</th>
                  <th className="py-3 px-4">Duration</th>
                  <th className="py-3 px-4 text-right">Total / Unique</th>
                  <th className="py-3 px-4">Errors / Notes</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800/60 text-slate-200">
                {runs.map((r) => (
                  <tr
                    key={r.run_id}
                    onClick={() => {
                      if (r.type === 'filesystem') {
                        navigate(`/runs/${r.run_id}/fs`)
                      } else {
                        navigate(`/runs/${r.run_id}/tls`)
                      }
                    }}
                    className="hover:bg-slate-800/40 cursor-pointer transition group"
                  >
                    <td className="py-3 px-4 font-mono text-xs text-cyan-300 font-semibold group-hover:text-cyan-200">
                      {r.run_id}
                    </td>
                    <td className="py-3 px-4">
                      <span className="flex items-center gap-1.5 capitalize font-medium text-xs text-slate-300">
                        {r.type === 'filesystem' ? (
                          <HardDrive className="w-3.5 h-3.5 text-sky-400" />
                        ) : (
                          <Network className="w-3.5 h-3.5 text-purple-400" />
                        )}
                        {r.type}
                      </span>
                    </td>
                    <td className="py-3 px-4 text-xs text-slate-400 capitalize">
                      {r.trigger}
                    </td>
                    <td className="py-3 px-4">
                      <StatusPill status={r.status} />
                    </td>
                    <td className="py-3 px-4 text-xs text-slate-400 font-mono">
                      {formatTimestamp(r.started_at || r.requested_at)}
                    </td>
                    <td className="py-3 px-4 text-xs text-slate-300 font-mono">
                      {formatDuration(r.duration_ms)}
                    </td>
                    <td className="py-3 px-4 text-xs font-mono text-right text-slate-300">
                      <span className="font-semibold">{r.records_total}</span>
                      <span className="text-slate-500"> / </span>
                      <span className="text-cyan-400">{r.records_unique}</span>
                    </td>
                    <td className="py-3 px-4 text-xs text-slate-400 max-w-xs truncate">
                      {r.error_summary ? (
                        <span className="text-amber-400 flex items-center gap-1" title={r.error_summary}>
                          <AlertTriangle className="w-3 h-3 shrink-0" />
                          <span className="truncate">{r.error_summary}</span>
                        </span>
                      ) : r.capture_note ? (
                        <span className="text-slate-400 truncate" title={r.capture_note}>
                          {r.capture_note}
                        </span>
                      ) : (
                        <span className="text-emerald-500/80 flex items-center gap-1">
                          <CheckCircle2 className="w-3 h-3 shrink-0" />
                          Clean
                        </span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </AsyncState>
    </div>
  )
}
