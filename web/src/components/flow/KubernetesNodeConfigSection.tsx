import type { NodeType, TemplateSuggestion } from '../../types'
import Input from '../ui/Input'
import Select from '../ui/Select'
import { Checkbox, Label } from '../ui/Form'
import { TemplateInput, TemplateTextarea } from '../ui/TemplateFields'

export const kubernetesNodeTypes = new Set<NodeType>([
  'action:kubernetes_api_resources',
  'action:kubernetes_list_resources',
  'action:kubernetes_get_resource',
  'action:kubernetes_apply_manifest',
  'action:kubernetes_patch_resource',
  'action:kubernetes_delete_resource',
  'action:kubernetes_scale_resource',
  'action:kubernetes_rollout_restart',
  'action:kubernetes_rollout_status',
  'action:kubernetes_pod_logs',
  'action:kubernetes_pod_exec',
  'action:kubernetes_events',
  'tool:kubernetes_api_resources',
  'tool:kubernetes_list_resources',
  'tool:kubernetes_get_resource',
  'tool:kubernetes_apply_manifest',
  'tool:kubernetes_patch_resource',
  'tool:kubernetes_delete_resource',
  'tool:kubernetes_scale_resource',
  'tool:kubernetes_rollout_restart',
  'tool:kubernetes_rollout_status',
  'tool:kubernetes_pod_logs',
  'tool:kubernetes_pod_exec',
  'tool:kubernetes_events',
])

export const kubernetesMutationNodeTypes = new Set<NodeType>([
  'action:kubernetes_apply_manifest',
  'action:kubernetes_patch_resource',
  'action:kubernetes_delete_resource',
  'action:kubernetes_scale_resource',
  'action:kubernetes_rollout_restart',
  'tool:kubernetes_apply_manifest',
  'tool:kubernetes_patch_resource',
  'tool:kubernetes_delete_resource',
  'tool:kubernetes_scale_resource',
  'tool:kubernetes_rollout_restart',
])

type KubernetesNodeConfigSectionProps = {
  nodeType: NodeType
  localConfig: Record<string, unknown>
  onConfigChange: (key: string, value: unknown) => void
  suggestions: TemplateSuggestion[]
  isToolNode: boolean
}

function commandToText(value: unknown): string {
  if (!Array.isArray(value)) {
    return ''
  }
  return value
    .map((item) => (typeof item === 'string' ? item.trim() : ''))
    .filter(Boolean)
    .join('\n')
}

function renderResourceFields(
  localConfig: Record<string, unknown>,
  onConfigChange: (key: string, value: unknown) => void,
  suggestions: TemplateSuggestion[],
  options: { includeName?: boolean; includeSelectors?: boolean; includeAllNamespaces?: boolean; includeLimit?: boolean; apiVersionPlaceholder?: string } = {},
) {
  return (
    <>
      <div>
        <Label>Namespace</Label>
        <TemplateInput value={(localConfig.namespace as string) || ''} onChange={(e) => onConfigChange('namespace', e.target.value)} placeholder="Optional namespace" suggestions={suggestions} />
      </div>
      <div>
        <Label>API Version</Label>
        <TemplateInput
          value={(localConfig.apiVersion as string) || ''}
          onChange={(e) => onConfigChange('apiVersion', e.target.value)}
          placeholder={options.apiVersionPlaceholder || 'v1 or apps/v1'}
          suggestions={suggestions}
        />
      </div>
      <div>
        <Label>Kind</Label>
        <TemplateInput value={(localConfig.kind as string) || ''} onChange={(e) => onConfigChange('kind', e.target.value)} placeholder="Deployment" suggestions={suggestions} />
      </div>
      <div>
        <Label>Resource</Label>
        <TemplateInput value={(localConfig.resource as string) || ''} onChange={(e) => onConfigChange('resource', e.target.value)} placeholder="deployments" suggestions={suggestions} />
      </div>
      {options.includeName && (
        <div>
          <Label>Name</Label>
          <TemplateInput value={(localConfig.name as string) || ''} onChange={(e) => onConfigChange('name', e.target.value)} placeholder="resource name" suggestions={suggestions} />
        </div>
      )}
      {options.includeSelectors && (
        <>
          <div>
            <Label>Label Selector</Label>
            <TemplateInput value={(localConfig.labelSelector as string) || ''} onChange={(e) => onConfigChange('labelSelector', e.target.value)} placeholder="app=web" suggestions={suggestions} />
          </div>
          <div>
            <Label>Field Selector</Label>
            <TemplateInput value={(localConfig.fieldSelector as string) || ''} onChange={(e) => onConfigChange('fieldSelector', e.target.value)} placeholder="status.phase=Running" suggestions={suggestions} />
          </div>
        </>
      )}
      {options.includeAllNamespaces && (
        <label className="flex items-start gap-3 rounded-lg border border-border bg-bg-input px-3 py-2">
          <Checkbox checked={Boolean(localConfig.allNamespaces)} onChange={(e) => onConfigChange('allNamespaces', e.target.checked)} className="mt-0.5" />
          <div className="min-w-0">
            <div className="text-sm font-medium text-text">All namespaces</div>
            <div className="mt-1 text-xs text-text-muted">Ignore the namespace field and search across the cluster.</div>
          </div>
        </label>
      )}
      {options.includeLimit && (
        <div>
          <Label>Limit</Label>
          <Input type="number" min="0" value={(localConfig.limit as number) ?? 0} onChange={(e) => onConfigChange('limit', parseInt(e.target.value, 10) || 0)} />
        </div>
      )}
    </>
  )
}

export default function KubernetesNodeConfigSection({
  nodeType,
  localConfig,
  onConfigChange,
  suggestions,
  isToolNode,
}: KubernetesNodeConfigSectionProps) {
  const commandText = commandToText(localConfig.command)

  return (
    <div className="space-y-4">
      {kubernetesMutationNodeTypes.has(nodeType) && (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-100">
          {isToolNode
            ? 'This tool can mutate Kubernetes resources. It only becomes available to an agent when explicitly connected.'
            : 'This node mutates Kubernetes resources. Double-check the target cluster, namespace, and resource fields before running the pipeline.'}
        </div>
      )}

      {nodeType === 'action:kubernetes_api_resources' || nodeType === 'tool:kubernetes_api_resources' ? (
        <div className="rounded-lg border border-border bg-bg-input px-3 py-2 text-sm text-text-muted">
          Discover the resources and verbs exposed by the selected cluster API server.
        </div>
      ) : nodeType === 'action:kubernetes_list_resources' || nodeType === 'tool:kubernetes_list_resources' ? (
        <>
          {renderResourceFields(localConfig, onConfigChange, suggestions, { includeSelectors: true, includeAllNamespaces: true, includeLimit: true })}
        </>
      ) : nodeType === 'action:kubernetes_get_resource' || nodeType === 'tool:kubernetes_get_resource' ? (
        <>{renderResourceFields(localConfig, onConfigChange, suggestions, { includeName: true })}</>
      ) : nodeType === 'action:kubernetes_apply_manifest' || nodeType === 'tool:kubernetes_apply_manifest' ? (
        <>
          <div>
            <Label>Namespace</Label>
            <TemplateInput value={(localConfig.namespace as string) || ''} onChange={(e) => onConfigChange('namespace', e.target.value)} placeholder="Optional namespace override" suggestions={suggestions} />
          </div>
          <div>
            <Label>Field Manager</Label>
            <Input value={(localConfig.fieldManager as string) || ''} onChange={(e) => onConfigChange('fieldManager', e.target.value)} placeholder="emerald" />
          </div>
          <label className="flex items-start gap-3 rounded-lg border border-border bg-bg-input px-3 py-2">
            <Checkbox checked={Boolean(localConfig.force)} onChange={(e) => onConfigChange('force', e.target.checked)} className="mt-0.5" />
            <div className="min-w-0">
              <div className="text-sm font-medium text-text">Force apply conflicts</div>
              <div className="mt-1 text-xs text-text-muted">Take ownership when server-side apply detects a field conflict.</div>
            </div>
          </label>
          <div>
            <Label>Manifest</Label>
            <TemplateTextarea value={(localConfig.manifest as string) || ''} onChange={(e) => onConfigChange('manifest', e.target.value)} rows={12} className="font-mono text-xs" placeholder="apiVersion: v1&#10;kind: ConfigMap&#10;..." suggestions={suggestions} />
          </div>
        </>
      ) : nodeType === 'action:kubernetes_patch_resource' || nodeType === 'tool:kubernetes_patch_resource' ? (
        <>
          {renderResourceFields(localConfig, onConfigChange, suggestions, { includeName: true, apiVersionPlaceholder: 'apps/v1' })}
          <div>
            <Label>Patch Type</Label>
            <Select value={(localConfig.patchType as string) || 'merge'} onChange={(e) => onConfigChange('patchType', e.target.value)}>
              <option value="merge">Merge</option>
              <option value="json">JSON Patch</option>
              <option value="strategic">Strategic Merge</option>
            </Select>
          </div>
          <div>
            <Label>Patch</Label>
            <TemplateTextarea value={(localConfig.patch as string) || ''} onChange={(e) => onConfigChange('patch', e.target.value)} rows={8} className="font-mono text-xs" placeholder='{"spec":{"replicas":3}}' suggestions={suggestions} />
          </div>
        </>
      ) : nodeType === 'action:kubernetes_delete_resource' || nodeType === 'tool:kubernetes_delete_resource' ? (
        <>
          {renderResourceFields(localConfig, onConfigChange, suggestions, { includeName: true, includeSelectors: true, apiVersionPlaceholder: 'apps/v1' })}
          <div>
            <Label>Propagation Policy</Label>
            <Select value={(localConfig.propagationPolicy as string) || 'background'} onChange={(e) => onConfigChange('propagationPolicy', e.target.value)}>
              <option value="background">Background</option>
              <option value="foreground">Foreground</option>
              <option value="orphan">Orphan</option>
            </Select>
          </div>
        </>
      ) : nodeType === 'action:kubernetes_scale_resource' || nodeType === 'tool:kubernetes_scale_resource' ? (
        <>
          {renderResourceFields(localConfig, onConfigChange, suggestions, { includeName: true, apiVersionPlaceholder: 'apps/v1' })}
          <div>
            <Label>Replicas</Label>
            <Input type="number" min="0" value={(localConfig.replicas as number) ?? 0} onChange={(e) => onConfigChange('replicas', parseInt(e.target.value, 10) || 0)} />
          </div>
        </>
      ) : nodeType === 'action:kubernetes_rollout_restart' || nodeType === 'tool:kubernetes_rollout_restart' ? (
        <>{renderResourceFields(localConfig, onConfigChange, suggestions, { includeName: true, apiVersionPlaceholder: 'apps/v1' })}</>
      ) : nodeType === 'action:kubernetes_rollout_status' || nodeType === 'tool:kubernetes_rollout_status' ? (
        <>
          {renderResourceFields(localConfig, onConfigChange, suggestions, { includeName: true, apiVersionPlaceholder: 'apps/v1' })}
          <div>
            <Label>Timeout Seconds</Label>
            <Input type="number" min="0" value={(localConfig.timeoutSeconds as number) ?? 300} onChange={(e) => onConfigChange('timeoutSeconds', parseInt(e.target.value, 10) || 0)} />
          </div>
        </>
      ) : nodeType === 'action:kubernetes_pod_logs' || nodeType === 'tool:kubernetes_pod_logs' ? (
        <>
          <div>
            <Label>Namespace</Label>
            <TemplateInput value={(localConfig.namespace as string) || ''} onChange={(e) => onConfigChange('namespace', e.target.value)} placeholder="Optional namespace" suggestions={suggestions} />
          </div>
          <div>
            <Label>Pod Name</Label>
            <TemplateInput value={(localConfig.name as string) || ''} onChange={(e) => onConfigChange('name', e.target.value)} placeholder="pod name" suggestions={suggestions} />
          </div>
          <div>
            <Label>Container</Label>
            <TemplateInput value={(localConfig.container as string) || ''} onChange={(e) => onConfigChange('container', e.target.value)} placeholder="Optional container" suggestions={suggestions} />
          </div>
          <div>
            <Label>Tail Lines</Label>
            <Input type="number" min="0" value={(localConfig.tailLines as number) ?? 0} onChange={(e) => onConfigChange('tailLines', parseInt(e.target.value, 10) || 0)} />
          </div>
          <div>
            <Label>Since Seconds</Label>
            <Input type="number" min="0" value={(localConfig.sinceSeconds as number) ?? 0} onChange={(e) => onConfigChange('sinceSeconds', parseInt(e.target.value, 10) || 0)} />
          </div>
          <label className="flex items-start gap-3 rounded-lg border border-border bg-bg-input px-3 py-2">
            <Checkbox checked={Boolean(localConfig.timestamps)} onChange={(e) => onConfigChange('timestamps', e.target.checked)} className="mt-0.5" />
            <div className="min-w-0">
              <div className="text-sm font-medium text-text">Include timestamps</div>
            </div>
          </label>
          <label className="flex items-start gap-3 rounded-lg border border-border bg-bg-input px-3 py-2">
            <Checkbox checked={Boolean(localConfig.previous)} onChange={(e) => onConfigChange('previous', e.target.checked)} className="mt-0.5" />
            <div className="min-w-0">
              <div className="text-sm font-medium text-text">Previous container instance</div>
            </div>
          </label>
        </>
      ) : nodeType === 'action:kubernetes_pod_exec' || nodeType === 'tool:kubernetes_pod_exec' ? (
        <>
          <div>
            <Label>Namespace</Label>
            <TemplateInput value={(localConfig.namespace as string) || ''} onChange={(e) => onConfigChange('namespace', e.target.value)} placeholder="Optional namespace" suggestions={suggestions} />
          </div>
          <div>
            <Label>Pod Name</Label>
            <TemplateInput value={(localConfig.name as string) || ''} onChange={(e) => onConfigChange('name', e.target.value)} placeholder="pod name" suggestions={suggestions} />
          </div>
          <div>
            <Label>Container</Label>
            <TemplateInput value={(localConfig.container as string) || ''} onChange={(e) => onConfigChange('container', e.target.value)} placeholder="Optional container" suggestions={suggestions} />
          </div>
          <div>
            <Label>Command</Label>
            <TemplateTextarea
              value={commandText}
              onChange={(e) => onConfigChange('command', e.target.value.split(/\r?\n/).map((value) => value.trim()).filter(Boolean))}
              rows={6}
              className="font-mono text-xs"
              placeholder={'kubectl style args, one per line\nsh\n-c\nprintenv'}
              suggestions={suggestions}
            />
          </div>
        </>
      ) : nodeType === 'action:kubernetes_events' || nodeType === 'tool:kubernetes_events' ? (
        <>
          <div>
            <Label>Namespace</Label>
            <TemplateInput value={(localConfig.namespace as string) || ''} onChange={(e) => onConfigChange('namespace', e.target.value)} placeholder="Optional namespace" suggestions={suggestions} />
          </div>
          <div>
            <Label>Limit</Label>
            <Input type="number" min="0" value={(localConfig.limit as number) ?? 0} onChange={(e) => onConfigChange('limit', parseInt(e.target.value, 10) || 0)} />
          </div>
          <div>
            <Label>Field Selector</Label>
            <TemplateInput value={(localConfig.fieldSelector as string) || ''} onChange={(e) => onConfigChange('fieldSelector', e.target.value)} placeholder="reason=FailedScheduling" suggestions={suggestions} />
          </div>
          <div>
            <Label>Involved Object Name</Label>
            <TemplateInput value={(localConfig.involvedObjectName as string) || ''} onChange={(e) => onConfigChange('involvedObjectName', e.target.value)} placeholder="Optional object name" suggestions={suggestions} />
          </div>
          <div>
            <Label>Involved Object Kind</Label>
            <TemplateInput value={(localConfig.involvedObjectKind as string) || ''} onChange={(e) => onConfigChange('involvedObjectKind', e.target.value)} placeholder="Optional object kind" suggestions={suggestions} />
          </div>
          <div>
            <Label>Involved Object UID</Label>
            <TemplateInput value={(localConfig.involvedObjectUID as string) || ''} onChange={(e) => onConfigChange('involvedObjectUID', e.target.value)} placeholder="Optional object UID" suggestions={suggestions} />
          </div>
        </>
      ) : null}
    </div>
  )
}
