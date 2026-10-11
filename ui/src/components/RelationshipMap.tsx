import React, { useState, useEffect, useMemo } from 'react'
import {
  ReactFlow,
  Controls,
  Background,
  useNodesState,
  useEdgesState,
  Node,
  Edge,
  MarkerType,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import dagre from '@dagrejs/dagre'
import { GraphData, GraphNode, GraphEdge } from '../types'
import {
  Server,
  Cpu,
  Globe,
  ShieldAlert,
  ShieldCheck,
  X,
  Layers,
  Activity,
} from 'lucide-react'

interface Props {
  runId: string
  runGraph: GraphData | null
  globalGraph: GraphData | null
  isLoading?: boolean
}

const nodeWidth = 200
const nodeHeight = 65

function getLayoutedElements(nodes: Node[], edges: Edge[], direction = 'LR') {
  const dagreGraph = new dagre.graphlib.Graph()
  dagreGraph.setDefaultEdgeLabel(() => ({}))
  dagreGraph.setGraph({ rankdir: direction, ranksep: 80, nodesep: 30 })

  nodes.forEach((node) => {
    dagreGraph.setNode(node.id, { width: nodeWidth, height: nodeHeight })
  })

  edges.forEach((edge) => {
    dagreGraph.setEdge(edge.source, edge.target)
  })

  dagre.layout(dagreGraph)

  const newNodes = nodes.map((node) => {
    const nodeWithPosition = dagreGraph.node(node.id)
    return {
      ...node,
      position: {
        x: nodeWithPosition ? nodeWithPosition.x - nodeWidth / 2 : 0,
        y: nodeWithPosition ? nodeWithPosition.y - nodeHeight / 2 : 0,
      },
    }
  })

  return { nodes: newNodes, edges }
}

export const RelationshipMap: React.FC<Props> = ({
  runId,
  runGraph,
  globalGraph,
  isLoading,
}) => {
  const [scope, setScope] = useState<'run' | 'lifetime'>('run')
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null)

  const activeGraph = scope === 'run' ? runGraph : globalGraph

  // Build ReactFlow Nodes and Edges
  const { initialNodes, initialEdges, rawNodesMap } = useMemo(() => {
    if (!activeGraph) {
      return { initialNodes: [], initialEdges: [], rawNodesMap: new Map() }
    }

    const rawMap = new Map<string, GraphNode>()
    activeGraph.nodes.forEach((n) => rawMap.set(n.id, n))

    // Endpoint clutter collapse (>50 endpoints)
    const endpoints = activeGraph.nodes.filter((n) => n.type === 'endpoint')
    const collapseEndpoints = endpoints.length > 50

    const nodesToRender: Node[] = []
    const collapsedEndpointIds = new Set<string>()

    if (collapseEndpoints) {
      // Keep first 50, collapse the rest into "+N more"
      endpoints.slice(50).forEach((ep) => collapsedEndpointIds.add(ep.id))
      const clusterNode: Node = {
        id: 'cluster:more_endpoints',
        type: 'default',
        data: {
          label: (
            <div className="flex items-center gap-2 p-2 rounded-lg bg-slate-900 border border-slate-700 text-xs font-semibold text-cyan-400">
              <Globe className="w-4 h-4" />
              <span>+{endpoints.length - 50} more endpoints</span>
            </div>
          ),
        },
        position: { x: 0, y: 0 },
        draggable: false,
      }
      nodesToRender.push(clusterNode)
    }

    activeGraph.nodes.forEach((node) => {
      if (collapsedEndpointIds.has(node.id)) return

      let icon = <Server className="w-4 h-4 text-cyan-400" />
      let bg = 'bg-slate-900/90 border-slate-700'
      let ring = ''

      if (node.type === 'process') {
        icon = <Cpu className="w-4 h-4 text-purple-400" />
        bg = 'bg-slate-900/90 border-purple-800/60'
      } else if (node.type === 'endpoint') {
        icon = <Globe className="w-4 h-4 text-sky-400" />
        bg = 'bg-slate-900/90 border-sky-800/60'
      } else if (node.type === 'certificate') {
        // Risk styling: expired or classical certs get amber/red ring
        const isExpired = node.props?.expired || node.props?.validity === 'expired'
        if (isExpired) {
          icon = <ShieldAlert className="w-4 h-4 text-rose-400" />
          bg = 'bg-rose-950/40 border-rose-600'
          ring = 'ring-2 ring-rose-500'
        } else {
          icon = <ShieldCheck className="w-4 h-4 text-emerald-400" />
          bg = 'bg-slate-900/90 border-emerald-800/60'
        }
      }

      nodesToRender.push({
        id: node.id,
        type: 'default',
        data: {
          label: (
            <div
              onClick={() => setSelectedNode(node)}
              className={`flex items-center gap-2.5 p-2.5 rounded-lg border text-xs shadow-md ${bg} ${ring} cursor-pointer transition hover:scale-102`}
            >
              {icon}
              <div className="truncate text-left">
                <div className="font-semibold text-slate-200 truncate max-w-[130px]">
                  {node.label || node.id}
                </div>
                <div className="text-[10px] text-slate-400 capitalize">{node.type}</div>
              </div>
            </div>
          ),
        },
        position: { x: 0, y: 0 },
        draggable: false,
      })
    })

    const edgesToRender: Edge[] = []
    activeGraph.edges.forEach((edge) => {
      let target = edge.dst
      if (collapsedEndpointIds.has(edge.dst)) {
        target = 'cluster:more_endpoints'
      }

      const weight = edge.weight || 1
      const strokeWidth = Math.min(Math.max(Math.log(weight + 1) * 2, 1.5), 6)

      edgesToRender.push({
        id: edge.id,
        source: edge.src,
        target: target,
        label: edge.type,
        labelStyle: { fill: '#94a3b8', fontSize: 10, fontWeight: 500 },
        labelBgStyle: { fill: '#0f172a', fillOpacity: 0.8 },
        style: { stroke: '#38bdf8', strokeWidth },
        markerEnd: { type: MarkerType.ArrowClosed, color: '#38bdf8' },
        animated: scope === 'run',
      })
    })

    const layouted = getLayoutedElements(nodesToRender, edgesToRender, 'LR')
    return {
      initialNodes: layouted.nodes,
      initialEdges: layouted.edges,
      rawNodesMap: rawMap,
    }
  }, [activeGraph, scope])

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)

  useEffect(() => {
    setNodes(initialNodes)
    setEdges(initialEdges)
  }, [initialNodes, initialEdges])

  return (
    <div className="rounded-xl border border-slate-800 bg-slate-950/60 overflow-hidden shadow-xl relative">
      {/* Header controls: This run | Lifetime */}
      <div className="p-3.5 border-b border-slate-800 flex items-center justify-between bg-slate-900/60">
        <div className="flex items-center gap-2 text-sm font-semibold text-slate-200">
          <Layers className="w-4 h-4 text-cyan-400" />
          <span>Cryptographic Relationship Map</span>
          <span className="text-xs text-slate-400 font-normal ml-2">
            (Host → Process → Endpoint → Certificate)
          </span>
        </div>

        <div className="flex items-center gap-1 bg-slate-950 p-1 rounded-lg border border-slate-800 text-xs">
          <button
            onClick={() => setScope('run')}
            className={`px-3 py-1 rounded font-medium transition cursor-pointer ${
              scope === 'run'
                ? 'bg-cyan-600 text-white shadow'
                : 'text-slate-400 hover:text-slate-200'
            }`}
          >
            This Run
          </button>
          <button
            onClick={() => setScope('lifetime')}
            className={`px-3 py-1 rounded font-medium transition cursor-pointer ${
              scope === 'lifetime'
                ? 'bg-cyan-600 text-white shadow'
                : 'text-slate-400 hover:text-slate-200'
            }`}
          >
            Lifetime
          </button>
        </div>
      </div>

      {/* Map Canvas */}
      <div className="h-[450px] w-full bg-slate-950 relative">
        {isLoading ? (
          <div className="h-full flex items-center justify-center text-slate-500 text-sm">
            Loading graph relationships...
          </div>
        ) : nodes.length === 0 ? (
          <div className="h-full flex items-center justify-center text-slate-500 text-sm">
            No relationship edges recorded yet for this view.
          </div>
        ) : (
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            fitView
            proOptions={{ hideAttribution: true }}
          >
            <Background color="#1e293b" gap={16} />
            <Controls className="bg-slate-900 border border-slate-700 fill-slate-300" />
          </ReactFlow>
        )}
      </div>

      {/* Node Details Drawer */}
      {selectedNode && (
        <div className="absolute top-0 right-0 bottom-0 w-80 bg-slate-950/95 border-l border-slate-800 p-5 shadow-2xl z-20 overflow-y-auto space-y-4">
          <div className="flex items-center justify-between pb-3 border-b border-slate-800">
            <div className="font-semibold text-slate-200 text-sm capitalize">
              {selectedNode.type} Node Details
            </div>
            <button
              onClick={() => setSelectedNode(null)}
              className="text-slate-400 hover:text-white"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="space-y-2 text-xs">
            <div>
              <span className="text-slate-400 block">Node ID</span>
              <span className="font-mono text-cyan-300 break-all">{selectedNode.id}</span>
            </div>
            <div>
              <span className="text-slate-400 block">Label</span>
              <span className="font-medium text-slate-200">{selectedNode.label || '—'}</span>
            </div>
            {selectedNode.first_seen && (
              <div>
                <span className="text-slate-400 block">First Observed</span>
                <span className="text-slate-300">{new Date(selectedNode.first_seen).toLocaleString()}</span>
              </div>
            )}
            {selectedNode.last_seen && (
              <div>
                <span className="text-slate-400 block">Last Observed</span>
                <span className="text-slate-300">{new Date(selectedNode.last_seen).toLocaleString()}</span>
              </div>
            )}
          </div>

          {selectedNode.props && Object.keys(selectedNode.props).length > 0 && (
            <div className="pt-2 border-t border-slate-800/80 space-y-1.5 text-xs">
              <span className="font-semibold text-slate-400 uppercase tracking-wider text-[10px]">
                Properties & Metadata
              </span>
              <pre className="p-2.5 rounded bg-slate-900 border border-slate-800 text-[11px] font-mono text-slate-300 overflow-x-auto">
                {JSON.stringify(selectedNode.props, null, 2)}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
