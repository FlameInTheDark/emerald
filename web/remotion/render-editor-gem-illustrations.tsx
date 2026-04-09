import fs from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

import { EditorGemNetwork } from './EditorGemNetwork'

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url))
const outputDirectory = path.resolve(scriptDirectory, '../../assets')

const renders = [
  {
    filename: 'node-editor-gem-network.svg',
    transparent: false,
  },
  {
    filename: 'node-editor-gem-network-transparent.svg',
    transparent: true,
  },
] as const

async function main() {
  await fs.mkdir(outputDirectory, { recursive: true })

  for (const render of renders) {
    const markup = renderToStaticMarkup(
      <EditorGemNetwork transparent={render.transparent} />,
    )
    const fileContents = `<?xml version="1.0" encoding="UTF-8"?>\n${markup}\n`
    const outputPath = path.join(outputDirectory, render.filename)

    await fs.writeFile(outputPath, fileContents, 'utf8')
    console.log(`Generated ${outputPath}`)
  }
}

void main().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
