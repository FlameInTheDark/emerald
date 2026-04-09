import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '../../api/client'
import { ASSISTANT_MODULES, ASSISTANT_SCOPE_LABELS } from '../../lib/assistantProfiles'
import { useUIStore } from '../../store/ui'
import Button from '../ui/Button'
import Skeleton from '../ui/Skeleton'
import { Card, CardContent } from '../ui/Card'
import { Checkbox, Label, Textarea } from '../ui/Form'
import { cn } from '../../lib/utils'
import type { AssistantModuleId, AssistantProfile, AssistantProfileScope } from '../../types'

type ProfileDraft = {
  system_instructions: string
  enabled_modules: AssistantModuleId[]
}

const PROFILE_SCOPES: AssistantProfileScope[] = ['pipeline_editor', 'chat_window']

function normalizeEnabledModules(value: AssistantModuleId[] | null | undefined): AssistantModuleId[] {
  return Array.isArray(value) ? value.filter((item): item is AssistantModuleId => typeof item === 'string') : []
}

function createDraft(profile: AssistantProfile): ProfileDraft {
  return {
    system_instructions: profile.system_instructions ?? '',
    enabled_modules: normalizeEnabledModules(profile.enabled_modules),
  }
}

export default function AssistantProfilesSettings() {
  const queryClient = useQueryClient()
  const { addToast } = useUIStore()
  const [activeScope, setActiveScope] = useState<AssistantProfileScope>('pipeline_editor')
  const [drafts, setDrafts] = useState<Partial<Record<AssistantProfileScope, ProfileDraft>>>({})

  const pipelineEditorProfileQuery = useQuery<AssistantProfile>({
    queryKey: ['assistant-profile', 'pipeline_editor'],
    queryFn: () => api.assistantProfiles.get('pipeline_editor'),
  })

  const chatWindowProfileQuery = useQuery<AssistantProfile>({
    queryKey: ['assistant-profile', 'chat_window'],
    queryFn: () => api.assistantProfiles.get('chat_window'),
  })

  const profileMap = useMemo(() => ({
    pipeline_editor: pipelineEditorProfileQuery.data,
    chat_window: chatWindowProfileQuery.data,
  }), [chatWindowProfileQuery.data, pipelineEditorProfileQuery.data])

  useEffect(() => {
    for (const scope of PROFILE_SCOPES) {
      const profile = profileMap[scope]
      if (!profile) {
        continue
      }

      setDrafts((current) => (
        current[scope]
          ? current
          : {
              ...current,
              [scope]: createDraft(profile),
            }
      ))
    }
  }, [profileMap])

  const saveMutation = useMutation({
    mutationFn: async ({ scope, draft }: { scope: AssistantProfileScope; draft: ProfileDraft }) =>
      api.assistantProfiles.update(scope, draft),
    onSuccess: (profile) => {
      queryClient.setQueryData<AssistantProfile>(['assistant-profile', profile.scope], profile)
      setDrafts((current) => ({
        ...current,
        [profile.scope]: createDraft(profile),
      }))
      addToast({ type: 'success', title: 'Assistant profile saved' })
    },
    onError: (error) => {
      addToast({
        type: 'error',
        title: 'Failed to save assistant profile',
        message: error instanceof Error ? error.message : 'Unknown error',
      })
    },
  })

  const restoreMutation = useMutation({
    mutationFn: async (scope: AssistantProfileScope) => api.assistantProfiles.restoreDefaults(scope),
    onSuccess: (profile) => {
      queryClient.setQueryData<AssistantProfile>(['assistant-profile', profile.scope], profile)
      setDrafts((current) => ({
        ...current,
        [profile.scope]: createDraft(profile),
      }))
      addToast({ type: 'success', title: 'Assistant profile restored' })
    },
    onError: (error) => {
      addToast({
        type: 'error',
        title: 'Failed to restore assistant profile',
        message: error instanceof Error ? error.message : 'Unknown error',
      })
    },
  })

  const currentDraft = drafts[activeScope]
  const isLoading = pipelineEditorProfileQuery.isLoading || chatWindowProfileQuery.isLoading

  function updateDraft(updater: (draft: ProfileDraft) => ProfileDraft) {
    setDrafts((current) => {
      const existing = current[activeScope] ?? {
        system_instructions: '',
        enabled_modules: [],
      }

      return {
        ...current,
        [activeScope]: updater(existing),
      }
    })
  }

  function handleToggleModule(moduleId: AssistantModuleId) {
    updateDraft((draft) => ({
      ...draft,
      enabled_modules: draft.enabled_modules.includes(moduleId)
        ? draft.enabled_modules.filter((id) => id !== moduleId)
        : draft.enabled_modules.concat(moduleId),
    }))
  }

  if (isLoading && !currentDraft) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-12 w-80 rounded-xl" />
        <Skeleton className="h-48 rounded-2xl" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-text">AI Assistants</h2>
        <p className="mt-1 text-sm text-text-muted">
          Edit the default instructions and built-in knowledge modules for the node editor assistant and the main chat window.
        </p>
      </div>

      <div className="overflow-x-auto pb-1 -mb-1">
        <div className="inline-flex min-w-max gap-1 rounded-xl border border-border bg-bg-input p-1">
          {PROFILE_SCOPES.map((scope) => (
            <button
              key={scope}
              type="button"
              onClick={() => setActiveScope(scope)}
              className={cn(
                'shrink-0 whitespace-nowrap rounded-lg px-4 py-2 text-sm font-medium transition-colors',
                activeScope === scope
                  ? 'bg-bg-elevated text-text shadow-sm'
                  : 'text-text-muted hover:text-text',
              )}
            >
              {ASSISTANT_SCOPE_LABELS[scope]}
            </button>
          ))}
        </div>
      </div>

      <Card className="overflow-hidden">
        <CardContent className="space-y-5 px-6 py-6">
          <div className="rounded-xl border border-border bg-bg-input/60 px-4 py-3 text-sm text-text-muted">
            {activeScope === 'pipeline_editor'
              ? 'Model selection stays in the node editor panel. These settings control the base prompt and built-in graph documentation.'
              : 'Model and infrastructure selections still live in each chat conversation. These settings control the shared chat prompt profile.'}
          </div>

          <div className="space-y-2">
            <Label htmlFor={`assistant-instructions-${activeScope}`}>Base Instructions</Label>
            <Textarea
              id={`assistant-instructions-${activeScope}`}
              rows={10}
              value={currentDraft?.system_instructions ?? ''}
              onChange={(event) => updateDraft((draft) => ({ ...draft, system_instructions: event.target.value }))}
              className="min-h-[16rem]"
            />
          </div>

          <div className="space-y-3">
            <div>
              <Label className="mb-0">Built-in Knowledge Modules</Label>
              <p className="mt-1 text-sm text-text-muted">
                Toggle the built-in graph and expression references that are injected into this assistant profile.
              </p>
            </div>

            <div className="space-y-3">
              {ASSISTANT_MODULES.map((module) => (
                <label
                  key={module.id}
                  className="flex items-start gap-3 rounded-xl border border-border bg-bg-input/50 px-4 py-3"
                >
                  <Checkbox
                    checked={Boolean(currentDraft?.enabled_modules?.includes(module.id))}
                    onChange={() => handleToggleModule(module.id)}
                  />
                  <div>
                    <div className="text-sm font-medium text-text">{module.name}</div>
                    <p className="mt-1 text-sm text-text-muted">{module.description}</p>
                  </div>
                </label>
              ))}
            </div>
          </div>

          <div className="flex flex-wrap items-center justify-end gap-3">
            <Button
              variant="ghost"
              onClick={() => restoreMutation.mutate(activeScope)}
              loading={restoreMutation.isPending && restoreMutation.variables === activeScope}
            >
              Restore Defaults
            </Button>
            <Button
              onClick={() => currentDraft && saveMutation.mutate({ scope: activeScope, draft: currentDraft })}
              loading={saveMutation.isPending && saveMutation.variables?.scope === activeScope}
              disabled={!currentDraft}
            >
              Save Profile
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
