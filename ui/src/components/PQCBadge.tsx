import React from 'react'
import { PQCClass } from '../types'

interface Props {
  pqcClass?: PQCClass | string | null
  className?: string
}

export const PQCBadge: React.FC<Props> = ({ pqcClass, className = '' }) => {
  const norm = (pqcClass || 'unknown').toLowerCase()

  switch (norm) {
    case 'classical':
      return (
        <span
          title="Quantum-vulnerable classical cryptography"
          className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-amber-950/50 text-amber-300 border border-amber-800/60 ${className}`}
        >
          Classical
        </span>
      )
    case 'hybrid':
      return (
        <span
          title="Post-Quantum + Classical hybrid"
          className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-cyan-950/50 text-cyan-300 border border-cyan-800/60 ${className}`}
        >
          Hybrid
        </span>
      )
    case 'pqc':
      return (
        <span
          title="Post-Quantum Cryptography"
          className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-emerald-950/50 text-emerald-300 border border-emerald-800/60 ${className}`}
        >
          PQC
        </span>
      )
    default:
      return (
        <span
          title="Unknown posture"
          className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-slate-800/50 text-slate-400 border border-slate-700/60 ${className}`}
        >
          Unknown
        </span>
      )
  }
}
