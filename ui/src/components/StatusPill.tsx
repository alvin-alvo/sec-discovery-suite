import React from 'react'
import { RunStatus } from '../types'

const statusConfig: Record<RunStatus, { label: string; bg: string; text: string; border: string; dot: string }> = {
  queued: {
    label: 'Queued',
    bg: 'bg-purple-950/40',
    text: 'text-purple-300',
    border: 'border-purple-800/60',
    dot: 'bg-purple-400',
  },
  running: {
    label: 'Running',
    bg: 'bg-sky-950/40',
    text: 'text-sky-300',
    border: 'border-sky-800/60',
    dot: 'bg-sky-400 animate-pulse',
  },
  completed: {
    label: 'Completed',
    bg: 'bg-emerald-950/40',
    text: 'text-emerald-300',
    border: 'border-emerald-800/60',
    dot: 'bg-emerald-400',
  },
  partial: {
    label: 'Partial',
    bg: 'bg-amber-950/40',
    text: 'text-amber-300',
    border: 'border-amber-800/60',
    dot: 'bg-amber-400',
  },
  failed: {
    label: 'Failed',
    bg: 'bg-rose-950/40',
    text: 'text-rose-300',
    border: 'border-rose-800/60',
    dot: 'bg-rose-400',
  },
}

export const StatusPill: React.FC<{ status: RunStatus; className?: string }> = ({ status, className = '' }) => {
  const conf = statusConfig[status] || {
    label: status,
    bg: 'bg-slate-800',
    text: 'text-slate-300',
    border: 'border-slate-700',
    dot: 'bg-slate-400',
  }

  return (
    <span
      className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium border ${conf.bg} ${conf.text} ${conf.border} ${className}`}
    >
      <span className={`w-1.5 h-1.5 rounded-full ${conf.dot}`} />
      {conf.label}
    </span>
  )
}
