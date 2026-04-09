import * as React from 'react'
import { Composition } from 'remotion'

import {
  EDITOR_GEM_ILLUSTRATION_HEIGHT,
  EDITOR_GEM_ILLUSTRATION_WIDTH,
  EditorGemNetwork,
  editorGemNetworkDefaults,
} from './EditorGemNetwork'

export function RemotionRoot() {
  return (
    <>
      <Composition
        id="EditorGemNetworkCanvas"
        component={EditorGemNetwork}
        durationInFrames={1}
        fps={30}
        width={EDITOR_GEM_ILLUSTRATION_WIDTH}
        height={EDITOR_GEM_ILLUSTRATION_HEIGHT}
        defaultProps={editorGemNetworkDefaults}
      />
      <Composition
        id="EditorGemNetworkTransparent"
        component={EditorGemNetwork}
        durationInFrames={1}
        fps={30}
        width={EDITOR_GEM_ILLUSTRATION_WIDTH}
        height={EDITOR_GEM_ILLUSTRATION_HEIGHT}
        defaultProps={{ transparent: true }}
      />
    </>
  )
}
