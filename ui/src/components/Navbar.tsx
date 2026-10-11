import React from 'react'
import { Link, useLocation } from 'react-router-dom'
import { Shield, Activity } from 'lucide-react'
import { AgentStatusChip } from './AgentStatusChip'

export const Navbar: React.FC = () => {
  const location = useLocation()

  return (
    <header className="sticky top-0 z-30 border-b border-slate-800 bg-slate-950/80 backdrop-blur-md">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between">
        <div className="flex items-center gap-8">
          <Link to="/runs" className="flex items-center gap-3 group">
            <div className="w-9 h-9 rounded-lg bg-gradient-to-tr from-cyan-600 to-sky-400 flex items-center justify-center text-white shadow-md shadow-cyan-900/30 group-hover:scale-105 transition">
              <Shield className="w-5 h-5" />
            </div>
            <div>
              <span className="font-bold text-base tracking-tight text-white block">
                Cryptographic Discovery Suite
              </span>
              <span className="text-[10px] uppercase tracking-wider text-slate-400 font-medium block -mt-0.5">
                Host & Network Cryptography Inventory
              </span>
            </div>
          </Link>

          <nav className="hidden md:flex items-center gap-1">
            <Link
              to="/runs"
              className={`px-3 py-1.5 rounded-md text-sm font-medium transition ${
                location.pathname === '/runs' || location.pathname === '/'
                  ? 'bg-slate-800/80 text-cyan-400 border border-slate-700/60'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900'
              }`}
            >
              <span className="flex items-center gap-1.5">
                <Activity className="w-4 h-4" />
                Discovery Runs
              </span>
            </Link>
          </nav>
        </div>

        <div className="flex items-center gap-4">
          <AgentStatusChip />
        </div>
      </div>
    </header>
  )
}
