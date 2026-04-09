import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Server, GitBranch, Clock, Activity, ExternalLink, Power } from 'lucide-react'
import { api } from '../api/client'
import { Card, CardContent } from '../components/ui/Card'
import Badge from '../components/ui/Badge'
import { formatDate } from '../lib/utils'
import type { Cluster, DashboardStats, Pipeline } from '../types'

export default function Dashboard() {
  const { data: clusters, isLoading } = useQuery<Cluster[]>({
    queryKey: ['clusters'],
    queryFn: () => api.clusters.list(),
  })
  const { data: dashboardStats } = useQuery<DashboardStats>({
    queryKey: ['dashboard-stats'],
    queryFn: () => api.dashboard.stats(),
  })
  const { data: pipelines } = useQuery<Pipeline[]>({
    queryKey: ['pipelines'],
    queryFn: () => api.pipelines.list(),
  })
  const activePipelines = (pipelines || []).filter((pipeline) => pipeline.status === 'active')

  const stats = [
    {
      label: 'Clusters',
      value: isLoading ? '...' : dashboardStats?.clusters ?? clusters?.length ?? 0,
      icon: Server,
      color: 'text-accent',
      bgColor: 'bg-accent/10',
    },
    {
      label: 'Pipelines',
      value: dashboardStats?.pipelines ?? 0,
      icon: GitBranch,
      color: 'text-blue-400',
      bgColor: 'bg-blue-400/10',
    },
    {
      label: 'Active Jobs',
      value: dashboardStats?.active_jobs ?? 0,
      icon: Clock,
      color: 'text-amber-400',
      bgColor: 'bg-amber-400/10',
    },
    {
      label: 'Executions (24h)',
      value: dashboardStats?.executions_24h ?? 0,
      icon: Activity,
      color: 'text-purple-400',
      bgColor: 'bg-purple-400/10',
    },
  ]

  return (
    <div className="p-6 max-w-7xl mx-auto">
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-text">Dashboard</h1>
        <p className="text-text-muted mt-1">Overview of your automation workspace</p>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        {stats.map(({ label, value, icon: Icon, color, bgColor }) => (
          <Card key={label}>
            <CardContent className="p-5">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-text-muted">{label}</p>
                  <p className="text-2xl font-bold text-text mt-1">{value}</p>
                </div>
                <div className={`w-10 h-10 rounded-lg ${bgColor} flex items-center justify-center`}>
                  <Icon className={`w-5 h-5 ${color}`} />
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      <Card className="mb-8">
        <CardContent className="p-0">
          <div className="flex items-center justify-between px-6 py-4 border-b border-border">
            <div>
              <h2 className="text-lg font-semibold text-text">Active Pipelines</h2>
              <p className="mt-1 text-sm text-text-muted">
                Pipelines with active triggers and scheduling enabled.
              </p>
            </div>
            <Badge variant="success">
              {dashboardStats?.active_pipelines ?? activePipelines.length}
            </Badge>
          </div>
          {activePipelines.length > 0 ? (
            <div className="divide-y divide-border">
              {activePipelines.map((pipeline) => (
                <div key={pipeline.id} className="flex items-center justify-between gap-4 px-6 py-4">
                  <div className="min-w-0 flex items-center gap-4">
                    <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-green-500/10">
                      <Power className="h-5 w-5 text-green-400" />
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <p className="truncate text-sm font-medium text-text">{pipeline.name}</p>
                        <Badge variant="success">active</Badge>
                      </div>
                      <p className="truncate text-xs text-text-dimmed">
                        {pipeline.description?.trim() || 'No description'}
                      </p>
                    </div>
                  </div>
                  <div className="flex flex-shrink-0 items-center gap-4">
                    <span className="text-xs text-text-dimmed">
                      Updated {formatDate(pipeline.updated_at, pipeline.created_at)}
                    </span>
                    <Link
                      to={`/pipelines/${pipeline.id}`}
                      className="inline-flex items-center gap-1 text-sm font-medium text-accent transition-colors hover:text-accent/80"
                    >
                      Open
                      <ExternalLink className="h-3.5 w-3.5" />
                    </Link>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="px-6 py-10 text-center">
              <GitBranch className="mx-auto mb-3 h-10 w-10 text-text-dimmed" />
              <p className="text-sm font-medium text-text">No active pipelines</p>
              <p className="mt-1 text-sm text-text-muted">
                Activate a pipeline to see it here and enable its triggers.
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      {clusters && clusters.length > 0 && (
        <Card>
          <CardContent className="p-0">
            <div className="px-6 py-4 border-b border-border">
              <h2 className="text-lg font-semibold text-text">Connected Clusters</h2>
            </div>
            <div className="divide-y divide-border">
              {clusters.map((cluster) => (
                <div key={cluster.id} className="px-6 py-4 flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <div className="w-10 h-10 rounded-lg bg-accent/10 flex items-center justify-center">
                      <Server className="w-5 h-5 text-accent" />
                    </div>
                    <div>
                      <p className="text-sm font-medium text-text">{cluster.name}</p>
                      <p className="text-xs text-text-dimmed">{cluster.host}:{cluster.port}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <span className="text-xs text-text-dimmed">
                      Added {formatDate(cluster.created_at)}
                    </span>
                    <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-green-600/20 text-green-400 border border-green-600/30">
                      <span className="w-1.5 h-1.5 rounded-full bg-green-400" />
                      Connected
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {clusters?.length === 0 && (
        <Card>
          <CardContent className="p-12 text-center">
            <Server className="w-12 h-12 text-text-dimmed mx-auto mb-4" />
            <h3 className="text-lg font-medium text-text mb-2">No clusters connected</h3>
            <p className="text-text-muted">Add your first cluster connection in Settings to get started</p>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
