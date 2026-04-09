import fs from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { bundle } from '@remotion/bundler'
import { renderStill, selectComposition } from '@remotion/renderer'

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url))
const entryPoint = path.join(scriptDirectory, 'index.ts')
const outputDirectory = path.resolve(scriptDirectory, '../../assets')
const outputPath = path.join(outputDirectory, 'node-editor-gem-network.png')

async function main() {
  await fs.mkdir(outputDirectory, { recursive: true })

  const bundledProject = await bundle({
    entryPoint,
    webpackOverride: (config) => config,
    onProgress: () => undefined,
  })

  const composition = await selectComposition({
    serveUrl: bundledProject,
    id: 'EditorGemNetworkCanvas',
    inputProps: {},
    logLevel: 'error',
  })

  await renderStill({
    composition,
    serveUrl: bundledProject,
    output: outputPath,
    imageFormat: 'png',
    inputProps: {},
    logLevel: 'error',
  })

  console.log(`Generated ${outputPath}`)
}

void main().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
