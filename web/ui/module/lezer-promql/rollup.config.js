import { nodeResolve } from "@rollup/plugin-node-resolve"
import typescript from "@rollup/plugin-typescript"

const clientTypescriptOptions = {
  tsconfig: false,
  include: ["src/client/**/*.ts"],
  compilerOptions: {
    target: "ES2020",
    module: "ESNext",
    moduleResolution: "node",
    declaration: false,
    strict: true,
    esModuleInterop: true,
    skipLibCheck: true,
    lib: ["ES2020", "DOM"]
  }
}

export default [
  // Parser bundle
  {
    input: "./src/parser.js",
    output: [{
      format: "cjs",
      file: "./dist/index.cjs"
    }, {
      format: "es",
      file: "./dist/index.es.js"
    }],
    external(id) { return !/^[.\/]/.test(id) },
    plugins: [
      nodeResolve()
    ]
  },
  // Client bundle (ESM)
  {
    input: "./src/client/index.ts",
    output: {
      format: "es",
      file: "./dist/esm/client/index.js"
    },
    external: ["lru-cache"],
    plugins: [
      typescript(clientTypescriptOptions),
      nodeResolve({ extensions: [".ts", ".js"] })
    ]
  },
  // Client bundle (CJS)
  {
    input: "./src/client/index.ts",
    output: {
      format: "cjs",
      file: "./dist/cjs/client/index.cjs"
    },
    external: ["lru-cache"],
    plugins: [
      typescript(clientTypescriptOptions),
      nodeResolve({ extensions: [".ts", ".js"] })
    ]
  }
]
