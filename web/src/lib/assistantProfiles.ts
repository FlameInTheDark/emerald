import type { AssistantModuleId, AssistantProfileScope } from '../types'

export const ASSISTANT_SCOPE_LABELS: Record<AssistantProfileScope, string> = {
  pipeline_editor: 'Node Editor',
  chat_window: 'Chat Window',
}

export const ASSISTANT_MODULES: Array<{
  id: AssistantModuleId
  name: string
  description: string
}> = [
  {
    id: 'pipeline_graph_rules',
    name: 'Pipeline Graph Rules',
    description: 'Validation rules for legal node and edge structures.',
  },
  {
    id: 'node_catalog',
    name: 'Node Catalog',
    description: 'Compact reference for the available pipeline node categories.',
  },
  {
    id: 'templating_guide',
    name: 'Templating Guide',
    description: 'How {{template}} interpolation works in node configuration.',
  },
  {
    id: 'logic_expression_guide',
    name: 'Logic Expression Guide',
    description: 'Rules and examples for condition and switch expressions.',
  },
  {
    id: 'llm_tool_edge_rules',
    name: 'LLM Tool Edge Rules',
    description: 'Connection rules for llm:agent tool nodes.',
  },
]
