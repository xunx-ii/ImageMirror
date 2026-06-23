import { Fragment } from "react"
import type { ReactNode } from "react"

function inline(text: string): ReactNode[] {
  const nodes: ReactNode[] = []
  const pattern = /!\[([^\]]*)\]\(([^)]+)\)|\[([^\]]+)\]\(([^)]+)\)|`([^`]+)`/g
  let index = 0
  let match: RegExpExecArray | null
  while ((match = pattern.exec(text))) {
    if (match.index > index) nodes.push(text.slice(index, match.index))
    if (match[1] !== undefined) {
      nodes.push(<img key={match.index} src={match[2]} alt={match[1]} className="my-3 max-h-[420px] rounded-lg object-contain" />)
    } else if (match[3] !== undefined) {
      nodes.push(
        <a key={match.index} className="underline underline-offset-4" href={match[4]} target="_blank" rel="noreferrer">
          {match[3]}
        </a>
      )
    } else if (match[5] !== undefined) {
      nodes.push(
        <code key={match.index} className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
          {match[5]}
        </code>
      )
    }
    index = pattern.lastIndex
  }
  if (index < text.length) nodes.push(text.slice(index))
  return nodes
}

export function renderMarkdown(markdown: string) {
  const lines = markdown.replace(/\r\n?/g, "\n").replace(/\\r\\n/g, "\n").replace(/\\n/g, "\n").split("\n")
  const output: ReactNode[] = []
  let list: string[] = []
  let code: string[] | null = null

  const flushList = () => {
    if (list.length === 0) return
    const items = list
    list = []
    output.push(
      <ul key={`list-${output.length}`} className="list-disc pl-5">
        {items.map((item, index) => (
          <li key={index}>{inline(item)}</li>
        ))}
      </ul>
    )
  }

  for (const line of lines) {
    if (line.startsWith("```")) {
      if (code) {
        output.push(
          <pre key={`code-${output.length}`} className="overflow-auto rounded-lg bg-muted p-3 text-xs">
            <code>{code.join("\n")}</code>
          </pre>
        )
        code = null
      } else {
        flushList()
        code = []
      }
      continue
    }
    if (code) {
      code.push(line)
      continue
    }
    if (line.startsWith("- ")) {
      list.push(line.slice(2))
      continue
    }
    flushList()
    if (!line.trim()) {
      output.push(<div key={`space-${output.length}`} className="h-2" />)
    } else if (line.startsWith("### ")) {
      output.push(
        <h3 key={output.length} className="text-base font-semibold">
          {inline(line.slice(4))}
        </h3>
      )
    } else if (line.startsWith("## ")) {
      output.push(
        <h2 key={output.length} className="text-lg font-semibold">
          {inline(line.slice(3))}
        </h2>
      )
    } else if (line.startsWith("# ")) {
      output.push(
        <h1 key={output.length} className="text-xl font-semibold">
          {inline(line.slice(2))}
        </h1>
      )
    } else {
      output.push(
        <p key={output.length} className="leading-7">
          {inline(line)}
        </p>
      )
    }
  }
  flushList()
  if (code) {
    output.push(
      <pre key={`code-${output.length}`} className="overflow-auto rounded-lg bg-muted p-3 text-xs">
        <code>{code.join("\n")}</code>
      </pre>
    )
  }
  return <Fragment>{output}</Fragment>
}
