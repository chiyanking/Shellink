import { defineConfig } from 'tsdown'

export default defineConfig({
  entry: {
    index: 'src/index.ts',
    tools: 'src/tools.ts',
  },
  dts: true,
  format: ['esm'],
  platform: 'node',
  fixedExtension: false,
  deps: {
    neverBundle: [/^@deepseek-ai\//, 'zod', '@msgpack/msgpack'],
    alwaysBundle: ['@shellink/protocol', '@shellink/client'],
  },
})
