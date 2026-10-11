import React from 'react'
import { Run } from '../types'
import { AlertTriangle, AlertCircle, RefreshCw, FileQuestion } from 'lucide-react'

interface Props {
  isLoading?: boolean
  error?: Error | null
  onRetry?: () => void
  isEmpty?: boolean
  emptyMessage?: string
  run?: Run | null
  children: React.ReactNode
}

export const AsyncState: React.FC<Props> = ({
  isLoading,
  error,
  onRetry,
  isEmpty,
  emptyMessage = 'No data available for this view.',
  run,
  children,
}) => {
  return (
    <div className="space-y-4">
      {/* 1. Persistent Sample Data Banner */}
      {run?.is_sample && (
        <div className="flex items-center gap-3 p-3 rounded-lg bg-amber-950/40 border border-amber-600/70 text-amber-200 text-sm">
          <AlertTriangle className="w-5 h-5 text-amber-400 shrink-0" />
          <div>
            <span className="font-semibold uppercase tracking-wider text-xs bg-amber-500/20 px-1.5 py-0.5 rounded mr-2 border border-amber-500/40">
              Sample Data
            </span>
            This run contains synthetic sample data loaded for demonstration purposes.
          </div>
        </div>
      )}

      {/* 2. Partial Status Banner */}
      {run?.status === 'partial' && (
        <div className="flex items-start gap-3 p-3.5 rounded-lg bg-amber-950/30 border border-amber-800/60 text-amber-200 text-sm">
          <AlertCircle className="w-5 h-5 text-amber-400 shrink-0 mt-0.5" />
          <div className="space-y-1">
            <div className="font-semibold text-amber-300">
              Partial Scan Coverage Recorded
            </div>
            <div className="text-amber-200/90 text-xs">
              {run.error_summary || 'Some targets could not be fully accessed during this scan.'}
            </div>
            {run.capture_note && (
              <div className="text-xs text-amber-400/80 font-mono">
                Capture note: {run.capture_note}
              </div>
            )}
          </div>
        </div>
      )}

      {/* 3. Loading Skeleton */}
      {isLoading ? (
        <div className="space-y-4 animate-pulse">
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            {[...Array(4)].map((_, i) => (
              <div key={i} className="h-24 bg-slate-800/40 rounded-xl border border-slate-700/40" />
            ))}
          </div>
          <div className="h-64 bg-slate-800/40 rounded-xl border border-slate-700/40" />
          <div className="h-96 bg-slate-800/40 rounded-xl border border-slate-700/40" />
        </div>
      ) : error ? (
        /* 4. Error State with Retry */
        <div className="flex flex-col items-center justify-center p-12 rounded-xl bg-rose-950/20 border border-rose-900/40 text-center">
          <AlertCircle className="w-12 h-12 text-rose-400 mb-3" />
          <h3 className="text-lg font-semibold text-rose-200">Failed to load data</h3>
          <p className="text-sm text-slate-400 max-w-md mt-1 mb-4">
            {error.message || 'An unexpected error occurred while communicating with the discovery API.'}
          </p>
          {onRetry && (
            <button
              onClick={onRetry}
              className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-rose-600/80 hover:bg-rose-500 text-white text-sm font-medium transition cursor-pointer"
            >
              <RefreshCw className="w-4 h-4" />
              Retry Request
            </button>
          )}
        </div>
      ) : isEmpty ? (
        /* 5. Empty State */
        <div className="flex flex-col items-center justify-center p-12 rounded-xl bg-slate-900/40 border border-slate-800 text-center">
          <FileQuestion className="w-12 h-12 text-slate-500 mb-3" />
          <h3 className="text-lg font-medium text-slate-300">No findings</h3>
          <p className="text-sm text-slate-400 max-w-md mt-1">{emptyMessage}</p>
        </div>
      ) : (
        /* 6. Normal content */
        children
      )}
    </div>
  )
}
