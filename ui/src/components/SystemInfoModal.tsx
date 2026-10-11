import React, { useState } from 'react'
import {
  X,
  Info,
  Shield,
  BookOpen,
  Terminal,
  Cpu,
  Lock,
  GitBranch,
  FileText,
  CheckCircle2,
  ExternalLink,
} from 'lucide-react'

interface SystemInfoModalProps {
  isOpen: boolean
  onClose: () => void
}

type TabKey = 'overview' | 'operations' | 'specs' | 'copyright'

export const SystemInfoModal: React.FC<SystemInfoModalProps> = ({ isOpen, onClose }) => {
  const [activeTab, setActiveTab] = useState<TabKey>('overview')

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-sm animate-fade-in">
      <div
        className="bg-slate-900 border border-slate-700/80 rounded-xl shadow-2xl max-w-4xl w-full max-h-[90vh] flex flex-col overflow-hidden text-slate-100"
        role="dialog"
        aria-modal="true"
        aria-labelledby="system-info-title"
      >
        {/* Modal Header */}
        <div className="px-6 py-4 border-b border-slate-800 bg-slate-950/60 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-cyan-600/20 border border-cyan-500/40 flex items-center justify-center text-cyan-400">
              <Info className="w-4 h-4" />
            </div>
            <div>
              <h2 id="system-info-title" className="text-lg font-bold text-white tracking-tight">
                Cryptographic Discovery Suite
              </h2>
              <p className="text-xs text-slate-400">
                Documentation, Operational Guide & Legal Attribution (v1.0.0)
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition"
            aria-label="Close dialog"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Tab Navigation */}
        <div className="px-6 border-b border-slate-800 bg-slate-900/60 flex gap-2 overflow-x-auto">
          <button
            onClick={() => setActiveTab('overview')}
            className={`py-3 px-3 text-sm font-medium border-b-2 flex items-center gap-2 transition whitespace-nowrap ${
              activeTab === 'overview'
                ? 'border-cyan-500 text-cyan-400'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <BookOpen className="w-4 h-4" />
            What is Happening Here
          </button>
          <button
            onClick={() => setActiveTab('operations')}
            className={`py-3 px-3 text-sm font-medium border-b-2 flex items-center gap-2 transition whitespace-nowrap ${
              activeTab === 'operations'
                ? 'border-cyan-500 text-cyan-400'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Terminal className="w-4 h-4" />
            How to Use & Operate
          </button>
          <button
            onClick={() => setActiveTab('specs')}
            className={`py-3 px-3 text-sm font-medium border-b-2 flex items-center gap-2 transition whitespace-nowrap ${
              activeTab === 'specs'
                ? 'border-cyan-500 text-cyan-400'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Cpu className="w-4 h-4" />
            Technical Architecture
          </button>
          <button
            onClick={() => setActiveTab('copyright')}
            className={`py-3 px-3 text-sm font-medium border-b-2 flex items-center gap-2 transition whitespace-nowrap ${
              activeTab === 'copyright'
                ? 'border-cyan-500 text-cyan-400'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <FileText className="w-4 h-4" />
            Copyright & License
          </button>
        </div>

        {/* Tab Content Body */}
        <div className="p-6 overflow-y-auto space-y-6 text-sm leading-relaxed text-slate-300">
          {activeTab === 'overview' && (
            <div className="space-y-5">
              <div className="p-4 rounded-lg bg-cyan-950/30 border border-cyan-800/40">
                <h3 className="text-base font-semibold text-cyan-300 mb-1 flex items-center gap-2">
                  <Shield className="w-4 h-4 text-cyan-400" />
                  Purpose of the Suite
                </h3>
                <p className="text-xs text-slate-300">
                  Cryptographic Discovery Suite provides automated, non-invasive discovery of cryptographic assets across hosts and networks. It continuously identifies certificates, keys, cipher suites, protocol versions, and cryptographic relationships without decrypting traffic or impacting performance.
                </p>
              </div>

              <div>
                <h4 className="text-sm font-semibold text-white mb-2 flex items-center gap-2">
                  <Lock className="w-4 h-4 text-emerald-400" />
                  Zero-Decryption & Security Assurance
                </h4>
                <ul className="space-y-1.5 text-xs list-disc list-inside text-slate-300">
                  <li><strong>Zero Payload Decryption:</strong> Never intercepts, decrypts, or records application layer data or TLS session payloads.</li>
                  <li><strong>Zero Private Key Exfiltration:</strong> Never reads, copies, or transmits private key bytes. Only public metadata, key algorithms, and parameters are cataloged.</li>
                  <li><strong>Passive Network Observation:</strong> Uses Linux raw packet sockets (<code className="text-cyan-300 bg-slate-800 px-1 py-0.5 rounded">AF_PACKET</code>) to parse cleartext TLS handshake headers (<code className="text-cyan-300 bg-slate-800 px-1 py-0.5 rounded">ClientHello</code>, <code className="text-cyan-300 bg-slate-800 px-1 py-0.5 rounded">ServerHello</code>) in real-time.</li>
                </ul>
              </div>

              <div>
                <h4 className="text-sm font-semibold text-white mb-2 flex items-center gap-2">
                  <GitBranch className="w-4 h-4 text-sky-400" />
                  Cryptographic Bill of Materials (CBOM) & Graph
                </h4>
                <p className="text-xs text-slate-300 mb-2">
                  The system correlates cryptographic primitives into a structured multi-tier topology graph:
                </p>
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
                  <div className="p-2.5 rounded-lg bg-slate-800/60 border border-slate-700/60 text-center">
                    <span className="text-[10px] uppercase font-bold text-slate-400 block">Layer 1</span>
                    <strong className="text-white block mt-0.5">Host System</strong>
                    <span className="text-[11px] text-slate-400">Agent nodes & OS</span>
                  </div>
                  <div className="p-2.5 rounded-lg bg-slate-800/60 border border-slate-700/60 text-center">
                    <span className="text-[10px] uppercase font-bold text-slate-400 block">Layer 2</span>
                    <strong className="text-white block mt-0.5">Active Process</strong>
                    <span className="text-[11px] text-slate-400">cURL, Nginx, Daemons</span>
                  </div>
                  <div className="p-2.5 rounded-lg bg-slate-800/60 border border-slate-700/60 text-center">
                    <span className="text-[10px] uppercase font-bold text-slate-400 block">Layer 3</span>
                    <strong className="text-white block mt-0.5">Network Endpoints</strong>
                    <span className="text-[11px] text-slate-400">IP, Port, SNI hostname</span>
                  </div>
                  <div className="p-2.5 rounded-lg bg-slate-800/60 border border-slate-700/60 text-center">
                    <span className="text-[10px] uppercase font-bold text-slate-400 block">Layer 4</span>
                    <strong className="text-white block mt-0.5">X.509 Certificate</strong>
                    <span className="text-[11px] text-slate-400">Validity, SANs, Alg</span>
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'operations' && (
            <div className="space-y-4">
              <div>
                <h4 className="text-sm font-semibold text-white mb-2 flex items-center gap-2">
                  <Terminal className="w-4 h-4 text-cyan-400" />
                  1. Launching Agent & Server
                </h4>
                <div className="space-y-2 text-xs">
                  <p className="text-slate-300">Start the lightweight agent (requires root/CAP_NET_RAW for AF_PACKET packet sniffing):</p>
                  <pre className="p-3 bg-slate-950 rounded-lg border border-slate-800 text-cyan-300 font-mono overflow-x-auto">
                    sudo ./bin/sds-agent serve --bind 127.0.0.1:7471
                  </pre>
                  <p className="text-slate-300 mt-2">Start the management server and UI dashboard:</p>
                  <pre className="p-3 bg-slate-950 rounded-lg border border-slate-800 text-cyan-300 font-mono overflow-x-auto">
                    ./bin/sds-server --bind 127.0.0.1:8080 --agent http://127.0.0.1:7471
                  </pre>
                </div>
              </div>

              <div>
                <h4 className="text-sm font-semibold text-white mb-2 flex items-center gap-2">
                  <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                  2. Executing Discovery Runs
                </h4>
                <ul className="space-y-2 text-xs text-slate-300 list-disc list-inside">
                  <li>
                    <strong>Filesystem Scans:</strong> Click &ldquo;New Discovery Run&rdquo; &rarr; select &ldquo;Filesystem Discovery&rdquo;. You can accept default trust store paths (<code className="text-cyan-300 bg-slate-800 px-1 py-0.5 rounded">/etc/ssl, /usr/share/ca-certificates</code>) or specify custom directories. Results stream dynamically into the report.
                  </li>
                  <li>
                    <strong>TLS Traffic Sniffing:</strong> Select &ldquo;TLS Traffic Sniffer&rdquo; &rarr; enter capture duration (e.g. 30s) and packet budget. Live flows will appear dynamically in the real-time activity stream as genuine network traffic traverses the network interfaces.
                  </li>
                </ul>
              </div>

              <div>
                <h4 className="text-sm font-semibold text-white mb-2 flex items-center gap-2">
                  <GitBranch className="w-4 h-4 text-sky-400" />
                  3. Interactive Relationship Map
                </h4>
                <p className="text-xs text-slate-300">
                  Open any completed TLS report to view the Cryptographic Relationship Map. You can zoom, pan, toggle full lifetime vs. single-run topology, and inspect nodes. Certificates expiring in &le;30 days or using deprecated algorithms are highlighted with prominent amber or red warning rings.
                </p>
              </div>
            </div>
          )}

          {activeTab === 'specs' && (
            <div className="space-y-4">
              <div>
                <h4 className="text-sm font-semibold text-white mb-2">Architectural Specifications</h4>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
                  <div className="p-3 rounded-lg bg-slate-800/40 border border-slate-700/60">
                    <strong className="text-cyan-300 block mb-1">Agent Footprint</strong>
                    <p className="text-slate-300">
                      Statically linked Go executable compressed with UPX (&lt;2.5 MB). Memory footprint &lt;15 MB RSS during peak scanning. Zero runtime dependencies.
                    </p>
                  </div>
                  <div className="p-3 rounded-lg bg-slate-800/40 border border-slate-700/60">
                    <strong className="text-cyan-300 block mb-1">Analytical Engine</strong>
                    <p className="text-slate-300">
                      Embedded in-process DuckDB columnar database. Instant multi-dimensional aggregation across certificates, algorithms, and cryptographic relationships.
                    </p>
                  </div>
                  <div className="p-3 rounded-lg bg-slate-800/40 border border-slate-700/60">
                    <strong className="text-cyan-300 block mb-1">Packet Sniffer Engine</strong>
                    <p className="text-slate-300">
                      Zero-copy Linux <code className="text-cyan-300 bg-slate-900 px-1 py-0.5 rounded">AF_PACKET</code> raw socket with <code className="text-cyan-300 bg-slate-900 px-1 py-0.5 rounded">htons(ETH_P_ALL)</code>. Decodes Ethernet, IPv4/IPv6, TCP, and TLS Record Layers.
                    </p>
                  </div>
                  <div className="p-3 rounded-lg bg-slate-800/40 border border-slate-700/60">
                    <strong className="text-cyan-300 block mb-1">Real-Time Streaming UI</strong>
                    <p className="text-slate-300">
                      React 18 + Vite frontend with dynamic infinite-scroll pagination, React Flow DAG visualizations, and SSE live telemetry channels.
                    </p>
                  </div>
                </div>
              </div>

              <div>
                <h4 className="text-sm font-semibold text-white mb-2">Supported Cryptographic Formats</h4>
                <div className="p-3 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono text-slate-300 space-y-1">
                  <div>• X.509 Certificates: PEM, DER, CRT, CER</div>
                  <div>• Key Algorithms: RSA (1024, 2048, 4096), ECDSA (P-256, P-384, P-521), Ed25519</div>
                  <div>• TLS Protocols: TLS 1.0, TLS 1.1, TLS 1.2, TLS 1.3</div>
                  <div>• Ciphers: AES-GCM, CHACHA20-POLY1305, AES-CBC, 3DES, RC4</div>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'copyright' && (
            <div className="space-y-4">
              <div className="p-4 rounded-lg bg-slate-950 border border-slate-800 font-mono text-xs text-slate-300 space-y-2">
                <p className="font-bold text-white">Cryptographic Discovery Suite</p>
                <p>
                  Copyright (C) 2026{' '}
                  <a
                    href="https://github.com/alvin-alvo/sec-discovery-suite"
                    target="_blank"
                    rel="noreferrer"
                    className="text-cyan-400 hover:underline inline-flex items-center gap-1 font-semibold"
                  >
                    Alvin Alvo
                    <ExternalLink className="w-3 h-3 inline" />
                  </a>
                  {' '}—{' '}
                  <a
                    href="https://github.com/alvin-alvo/sec-discovery-suite"
                    target="_blank"
                    rel="noreferrer"
                    className="text-slate-400 hover:text-cyan-300 underline"
                  >
                    https://github.com/alvin-alvo/sec-discovery-suite
                  </a>
                </p>
                <p className="text-slate-400">
                  This program is free software: you can redistribute it and/or modify
                  it under the terms of the GNU General Public License as published by
                  the Free Software Foundation, either version 3 of the License, or
                  (at your option) any later version.
                </p>
                <p className="text-slate-400">
                  This program is distributed in the hope that it will be useful,
                  but WITHOUT ANY WARRANTY; without even the implied warranty of
                  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
                  GNU General Public License for more details.
                </p>
                <p className="text-slate-400">
                  You should have received a copy of the GNU General Public License
                  along with this program. If not, see &lt;https://www.gnu.org/licenses/&gt;.
                </p>
              </div>

              <div>
                <h4 className="text-sm font-semibold text-white mb-2">Open-Source Attributions & Libraries</h4>
                <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 text-xs">
                  <div className="p-2 rounded bg-slate-800/40 border border-slate-700/60">
                    <span className="font-semibold text-slate-200">DuckDB</span>
                    <span className="block text-[11px] text-slate-400">MIT License</span>
                  </div>
                  <div className="p-2 rounded bg-slate-800/40 border border-slate-700/60">
                    <span className="font-semibold text-slate-200">React Flow / @xyflow</span>
                    <span className="block text-[11px] text-slate-400">MIT License</span>
                  </div>
                  <div className="p-2 rounded bg-slate-800/40 border border-slate-700/60">
                    <span className="font-semibold text-slate-200">TanStack Query</span>
                    <span className="block text-[11px] text-slate-400">MIT License</span>
                  </div>
                  <div className="p-2 rounded bg-slate-800/40 border border-slate-700/60">
                    <span className="font-semibold text-slate-200">Lucide Icons</span>
                    <span className="block text-[11px] text-slate-400">ISC License</span>
                  </div>
                  <div className="p-2 rounded bg-slate-800/40 border border-slate-700/60">
                    <span className="font-semibold text-slate-200">Tailwind CSS</span>
                    <span className="block text-[11px] text-slate-400">MIT License</span>
                  </div>
                  <div className="p-2 rounded bg-slate-800/40 border border-slate-700/60">
                    <span className="font-semibold text-slate-200">Dagre Layout</span>
                    <span className="block text-[11px] text-slate-400">MIT License</span>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Modal Footer */}
        <div className="px-6 py-3 border-t border-slate-800 bg-slate-950/60 flex items-center justify-between text-xs text-slate-400">
          <span>Release: v1.0.0-linux-amd64</span>
          <button
            onClick={onClose}
            className="px-4 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 font-medium transition"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
