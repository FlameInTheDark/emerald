import { useMutation, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'

import { api } from '../../api/client'
import { useNodeDefinitions } from '../../hooks/useNodeDefinitions'
import { useUIStore } from '../../store/ui'
import Badge from '../ui/Badge'
import Button from '../ui/Button'
import Skeleton from '../ui/Skeleton'
import { Card, CardContent } from '../ui/Card'

export default function PluginBundleSettings() {
  const queryClient = useQueryClient()
  const { addToast } = useUIStore()
  const { plugins, isLoading } = useNodeDefinitions()

  const refreshPluginsMutation = useMutation({
    mutationFn: () => api.nodeDefinitions.refresh(),
    onSuccess: (data) => {
      queryClient.setQueryData(['node-definitions'], data)
      if (data.error) {
        addToast({
          type: 'warning',
          title: 'Plugins rediscovered with issues',
          message: data.error,
        })
        return
      }

      addToast({
        type: 'success',
        title: 'Plugins rediscovered',
        message: 'Emerald reloaded local plugin bundles without a restart.',
      })
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Failed to rediscover plugins', message: err.message })
    },
  })

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold text-text">Plugins</h2>
          <p className="mt-1 text-sm text-text-muted">
            Inspect local plugin bundle health and refresh discoveries without restarting Emerald.
          </p>
        </div>
        <Button
          variant="secondary"
          onClick={() => refreshPluginsMutation.mutate()}
          loading={refreshPluginsMutation.isPending}
        >
          <RefreshCw className="w-4 h-4" />
          Rediscover Plugins
        </Button>
      </div>

      <Card>
        <CardContent className="space-y-4 pt-6">
          <div>
            <h3 className="text-base font-semibold text-text">Installed Plugin Bundles</h3>
            <p className="mt-1 text-sm text-text-muted">
              Emerald discovers local plugin bundles and surfaces their health here so missing or broken nodes are easier to spot.
            </p>
          </div>

          {isLoading && (
            <>
              <Skeleton className="h-20 w-full" />
              <Skeleton className="h-20 w-full" />
            </>
          )}

          {!isLoading && plugins.length === 0 && (
            <div className="rounded-xl border border-dashed border-border px-4 py-6 text-sm text-text-muted">
              No plugin bundles discovered yet. Add bundles under <code>.agents/plugins</code> or configure <code>EMERALD_PLUGINS_DIR</code>.
            </div>
          )}

          {!isLoading && plugins.map((plugin) => (
            <div
              key={plugin.id}
              className="rounded-xl border border-border bg-bg-overlay/40 px-4 py-4"
            >
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h4 className="truncate text-base font-semibold text-text">{plugin.name}</h4>
                    {plugin.version && <Badge variant="default">v{plugin.version}</Badge>}
                    <Badge variant={plugin.healthy ? 'success' : 'error'}>
                      {plugin.healthy ? 'Healthy' : 'Unavailable'}
                    </Badge>
                  </div>
                  {plugin.description && (
                    <p className="mt-1 text-sm text-text-muted">{plugin.description}</p>
                  )}
                  <p className="mt-1 text-xs text-text-dimmed">
                    {plugin.node_count} node{plugin.node_count === 1 ? '' : 's'} from {plugin.path}
                  </p>
                </div>
                {plugin.error && (
                  <div className="max-w-xl rounded-lg border border-red-600/30 bg-red-600/10 px-3 py-2 text-sm text-red-300">
                    {plugin.error}
                  </div>
                )}
              </div>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  )
}
