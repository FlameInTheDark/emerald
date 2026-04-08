import type { FlowDefinitionDocument, PipelineDocument, Pipeline, TemplateBundle, TemplateDocument } from '../types'

export const DOCUMENT_VERSION = 'v1'
export const PIPELINE_DOCUMENT_KIND = 'automator-pipeline'
export const TEMPLATE_DOCUMENT_KIND = 'automator-template'
export const TEMPLATE_BUNDLE_KIND = 'automator-template-bundle'

type ExtractedDocument = {
  kind: string
  name: string
  description?: string | null
  definition: FlowDefinitionDocument
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function parseDefinition(value: unknown): FlowDefinitionDocument {
  if (!isRecord(value)) {
    throw new Error('JSON document definition is missing.')
  }

  const nodes = value.nodes
  const edges = value.edges
  const viewport = value.viewport

  if (!Array.isArray(nodes)) {
    throw new Error('Definition nodes must be an array.')
  }
  if (!Array.isArray(edges)) {
    throw new Error('Definition edges must be an array.')
  }
  if (viewport !== undefined && !isRecord(viewport)) {
    throw new Error('Definition viewport must be an object when provided.')
  }

  return {
    nodes,
    edges,
    viewport: viewport as Record<string, unknown> | undefined,
  }
}

export function buildPipelineDocument(input: {
  name: string
  description?: string | null
  status?: Pipeline['status'] | string
  definition: FlowDefinitionDocument
}): PipelineDocument {
  return {
    version: DOCUMENT_VERSION,
    kind: PIPELINE_DOCUMENT_KIND,
    name: input.name,
    description: input.description ?? null,
    status: input.status,
    definition: input.definition,
  }
}

export function buildTemplateDocument(input: {
  name: string
  description?: string | null
  definition: FlowDefinitionDocument
}): TemplateDocument {
  return {
    version: DOCUMENT_VERSION,
    kind: TEMPLATE_DOCUMENT_KIND,
    name: input.name,
    description: input.description ?? null,
    definition: input.definition,
  }
}

export function extractSingleDefinitionDocument(value: unknown): ExtractedDocument {
  if (!isRecord(value)) {
    throw new Error('JSON document must be an object.')
  }

  if (Array.isArray(value.nodes) && Array.isArray(value.edges)) {
    return {
      kind: 'raw-definition',
      name: 'Imported JSON',
      definition: parseDefinition(value),
    }
  }

  const kind = typeof value.kind === 'string' ? value.kind : ''
  const name = typeof value.name === 'string' ? value.name : 'Imported JSON'
  const description = typeof value.description === 'string' ? value.description : null

  if (kind === PIPELINE_DOCUMENT_KIND || kind === TEMPLATE_DOCUMENT_KIND) {
    if (value.version !== DOCUMENT_VERSION) {
      throw new Error(`Unsupported JSON document version "${String(value.version)}".`)
    }

    return {
      kind,
      name,
      description,
      definition: parseDefinition(value.definition),
    }
  }

  if (kind === TEMPLATE_BUNDLE_KIND) {
    const templates = value.templates
    if (!Array.isArray(templates)) {
      throw new Error('Template bundle must contain a templates array.')
    }
    if (templates.length !== 1) {
      throw new Error('Template bundles with multiple templates should be imported from the Templates page.')
    }

    return extractSingleDefinitionDocument(templates[0])
  }

  throw new Error('Unsupported JSON document kind.')
}

export function isTemplateBundle(value: unknown): value is TemplateBundle {
  return isRecord(value) && value.kind === TEMPLATE_BUNDLE_KIND && Array.isArray(value.templates)
}
