import React from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api'

export const AgentStatusChip: React.FC = () => {
  const { data: status, isError } = useQuery({
    queryKey: ['agent-status'],
    queryFn: api.getAgentStatus,
    refetchInterval: 5000,
  })

  if (isError || !status || !status.online) {
    return (
      <div className="flex items-center gap-2 px-3 py-1 rounded-full bg-rose-950/40 border border-rose-800/60 text-xs text-rose-300">
        <span className="w-2 h-2 rounded-full bg-rose-500" />
        <span>Agent Offline</span>
      </div>
    )
  }

  const binarySizeMB = status.binary_bytes
    ? (status.binary_bytes / (1024 * 1024)).toFixed(2) + ' MB'
    : null

  const tiersText = status.tiers?.length ? status.tiers.join(', ') : 'af_packet'

  return (
    <div
      className="flex items-center gap-2.5 px-3 py-1 rounded-full bg-slate-900/80 border border-slate-700/60 text-xs text-slate-300 shadow-sm"
      title={`UID: ${status.uid ?? 'unknown'} | Privileged: ${status.privileged ? 'yes' : 'no'} | Tiers: ${tiersText}${binarySizeMB ? ` | Size: ${binarySizeMB}` : ''}`}
    >
      <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
      <span className="font-semibold text-slate-200">Agent {status.version || 'v0.1.0'}</span>
      <span className="text-slate-500">•</span>
      <span className="text-slate-400">
        {status.privileged ? 'privileged' : 'unprivileged'}
      </span>
      {binarySizeMB && (
        <>
          <span className="text-slate-500">•</span>
          <span className="text-slate-400">{binarySizeMB}</span>
        </>
      )}
    </div>
  )
}
