// graphify plugin for OpenCode V2.
//
// Graphify (0.9.64) ships a V1 plugin shape (`tool.execute.before`), which
// OpenCode V2 does not run. This is the same behaviour ported to the V2 plugin
// API: when the knowledge graph exists, the first shell command of a session
// carries a one-line reminder to query the graph instead of grepping raw files.
//
// IMPORTANT: keep REMINDER free of double quotes, backticks and $(...) — it is
// embedded inside a double-quoted `echo` command.
import { Plugin } from "@opencode/plugin"
import { existsSync } from "node:fs"
import { join } from "node:path"

const REMINDER =
  "[graphify] knowledge graph at graphify-out/. For focused questions run graphify query with your question (scoped subgraph, usually much smaller than the report) instead of grepping raw files. Read graphify-out/GRAPH_REPORT.md for broad architecture context."

export default Plugin.define({
  id: "graphify",
  async setup(ctx) {
    const graph = join(ctx.location.directory, "graphify-out", "graph.json")
    if (!existsSync(graph)) return

    let reminded = false
    await ctx.shell.hook("create.before", (event) => {
      if (reminded) return
      reminded = true
      // ';' not '&&' — Windows PowerShell 5.1 rejects '&&' as a separator.
      event.command = `echo "${REMINDER}" ; ${event.command}`
    })
  },
})
