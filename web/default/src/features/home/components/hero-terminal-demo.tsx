/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState, useEffect, useRef, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

type AccentTone = 'emerald' | 'amber' | 'blue' | 'violet'

interface ApiDemoConfig {
  id: string
  label: string
  method: 'POST' | 'GET'
  endpoint: string
  headers: string[]
  request: string[]
  response: string[]
  responseHighlights: string[]
  tokens: number
  latency: number
  accent: AccentTone
}

const ACCENT_CLASSES: Record<
  AccentTone,
  {
    tone: string
    toneSoft: string
    toneDark: string
  }
> = {
  emerald: {
    tone: '#60915d',
    toneSoft: '#e0eddc',
    toneDark: '#5f895c',
  },
  amber: {
    tone: '#bd7d3f',
    toneSoft: '#fce4cf',
    toneDark: '#8a5a2a',
  },
  blue: {
    tone: '#2f86ad',
    toneSoft: '#def0fa',
    toneDark: '#2d6e89',
  },
  violet: {
    tone: '#7760a8',
    toneSoft: '#e8e1f5',
    toneDark: '#6e5e9e',
  },
}

const API_DEMOS: ApiDemoConfig[] = [
  {
    id: 'gpt-chat',
    label: 'Chat',
    method: 'POST',
    endpoint: '/v1/chat/completions',
    headers: ['"Authorization: Bearer sk-••••"'],
    request: [
      '"model": "your-model",',
      '"messages": [',
      '  { "role": "user", "content": "..." }',
      ']',
    ],
    response: [
      '{',
      '  "choices": [{ "message": { "content": <text> } }],',
      '  "usage": { "total_tokens": <tokens> }',
      '}',
    ],
    responseHighlights: ['<text>', '<tokens>'],
    tokens: 27,
    latency: 142,
    accent: 'emerald',
  },
  {
    id: 'responses',
    label: 'Responses',
    method: 'POST',
    endpoint: '/v1/responses',
    headers: ['"Authorization: Bearer sk-••••"'],
    request: ['"model": "your-model",', '"input": "..."'],
    response: [
      '{',
      '  "output": [{ "type": "output_text", "text": <text> }],',
      '  "usage": { "total_tokens": <tokens> }',
      '}',
    ],
    responseHighlights: ['<text>', '<tokens>'],
    tokens: 31,
    latency: 168,
    accent: 'amber',
  },
  {
    id: 'claude',
    label: 'Claude',
    method: 'POST',
    endpoint: '/v1/messages',
    headers: ['"x-api-key: sk-••••"', '"anthropic-version: 2023-06-01"'],
    request: [
      '"model": "your-model",',
      '"max_tokens": 1024,',
      '"messages": [',
      '  { "role": "user", "content": "..." }',
      ']',
    ],
    response: [
      '{',
      '  "content": [{ "type": "text", "text": <text> }],',
      '  "usage": { "input_tokens": <in>, "output_tokens": <out> }',
      '}',
    ],
    responseHighlights: ['<text>', '<in>', '<out>'],
    tokens: 29,
    latency: 156,
    accent: 'blue',
  },
  {
    id: 'gemini',
    label: 'Gemini',
    method: 'POST',
    endpoint: '/v1beta/models/{model}:generateContent',
    headers: ['"x-goog-api-key: sk-••••"'],
    request: [
      '"contents": [',
      '  { "role": "user",',
      '    "parts": [{ "text": "..." }] }',
      ']',
    ],
    response: [
      '{',
      '  "candidates": [{ "content": { "parts": [{ "text": <text> }] } }],',
      '  "usageMetadata": { "totalTokenCount": <tokens> }',
      '}',
    ],
    responseHighlights: ['<text>', '<tokens>'],
    tokens: 25,
    latency: 93,
    accent: 'violet',
  },
]

const CYCLE_INTERVAL = 4500
const TRANSITION_MS = 220

interface HeroTerminalDemoProps {
  className?: string
}

export function HeroTerminalDemo(props: HeroTerminalDemoProps) {
  const [activeIndex, setActiveIndex] = useState(0)
  const [transitioning, setTransitioning] = useState(false)
  const intervalRef = useRef<ReturnType<typeof setInterval>>(undefined)
  const timeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    if (mq.matches) return

    intervalRef.current = setInterval(() => {
      setTransitioning(true)
      timeoutRef.current = setTimeout(() => {
        setActiveIndex((prev) => (prev + 1) % API_DEMOS.length)
        setTransitioning(false)
      }, TRANSITION_MS)
    }, CYCLE_INTERVAL)

    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
      if (timeoutRef.current) clearTimeout(timeoutRef.current)
    }
  }, [])

  const handleSelect = (index: number) => {
    if (index === activeIndex) return
    if (intervalRef.current) clearInterval(intervalRef.current)
    if (timeoutRef.current) clearTimeout(timeoutRef.current)
    setTransitioning(true)
    timeoutRef.current = setTimeout(() => {
      setActiveIndex(index)
      setTransitioning(false)
    }, TRANSITION_MS)
  }

  const demo = API_DEMOS[activeIndex]
  const accent = ACCENT_CLASSES[demo.accent]

  return (
    <div className={cn('home-terminal-wrap', props.className)}>
      <div
        className='home-terminal-card'
        style={
          {
            '--tone': accent.tone,
            '--tone-soft': accent.toneSoft,
            '--tone-dark': accent.toneDark,
          } as React.CSSProperties
        }
      >
        <div className='home-tab-strip' role='tablist' aria-label='API 示例'>
          {API_DEMOS.map((item, index) => {
            const tone = ACCENT_CLASSES[item.accent]
            const isActive = index === activeIndex
            return (
              <button
                key={item.id}
                type='button'
                role='tab'
                aria-selected={isActive}
                onClick={() => handleSelect(index)}
                className={cn('home-api-tab', isActive && 'active')}
                style={{ '--tone': tone.tone } as React.CSSProperties}
              >
                {item.label}
              </button>
            )
          })}
          <div className='home-terminal-status'>
            <span className='home-status-dot' />
            200 ok
          </div>
        </div>

        <div className='home-endpoint-row'>
          <span className='home-method'>{demo.method}</span>
          <code
            className={cn(
              'home-endpoint fade-swap',
              transitioning ? 'opacity-0' : 'opacity-100'
            )}
          >
            {demo.endpoint}
          </code>
        </div>

        <div className='home-terminal-body'>
          <RequestBlock demo={demo} transitioning={transitioning} />
          <ResponseBlock demo={demo} transitioning={transitioning} />
        </div>

        <div className='home-terminal-footer'>
          <div className='home-metrics'>
            <span>
              <strong>{demo.latency}</strong> ms
            </span>
            <span className='home-metric-dot' />
            <span>
              <strong>{demo.tokens}</strong> Token
            </span>
            <span className='home-metric-dot' />
            <span>
              费用 <strong>${(demo.tokens * 0.00003).toFixed(5)}</strong>
            </span>
          </div>
          <span>流式 · SSE</span>
        </div>
      </div>
    </div>
  )
}

function RequestBlock(props: { demo: ApiDemoConfig; transitioning: boolean }) {
  const { demo, transitioning } = props

  return (
    <div className='home-code-panel'>
      <SectionLabel>Request</SectionLabel>
      <div
        className={cn(
          'mt-2 transition-opacity duration-200',
          transitioning ? 'opacity-0' : 'opacity-100'
        )}
      >
        <CodeLine>
          <Command>curl</Command> <Flag>-X</Flag> <Flag>POST</Flag>{' '}
          <StringText>&quot;{demo.endpoint}&quot;</StringText>{' '}
          <Muted>{'\\'}</Muted>
        </CodeLine>
        {demo.headers.map((header) => (
          <CodeLine key={header} indent={2}>
            <Flag>-H</Flag> <StringText>{header}</StringText>{' '}
            <Muted>{'\\'}</Muted>
          </CodeLine>
        ))}
        <CodeLine indent={2}>
          <Flag>-d</Flag> <StringText>&apos;{'{'}</StringText>
        </CodeLine>
        {demo.request.map((line, i) => (
          <CodeLine key={i} indent={4}>
            {renderJsonLine(line)}
          </CodeLine>
        ))}
        <CodeLine indent={2}>
          <StringText>{'}'}&apos;</StringText>
        </CodeLine>
      </div>
    </div>
  )
}

function ResponseBlock(props: { demo: ApiDemoConfig; transitioning: boolean }) {
  const { demo, transitioning } = props

  return (
    <div className={cn('home-code-panel home-response-panel')}>
      <SectionLabel>Response</SectionLabel>
      <div
        className={cn(
          'mt-2 transition-opacity duration-200',
          transitioning ? 'opacity-0' : 'opacity-100'
        )}
      >
        {demo.response.map((line, i) => (
          <CodeLine key={i}>{renderResponseLine(line, demo)}</CodeLine>
        ))}
      </div>
    </div>
  )
}

function SectionLabel(props: { children: ReactNode }) {
  return <span className='home-section-label'>{props.children}</span>
}

const STRING_RE = /"[^"]*"/g
const PLACEHOLDER_RE = /<[a-z]+>/gi

function renderJsonLine(line: string): ReactNode {
  if (!line.trim()) return <Muted> </Muted>
  return tokenize(line)
}

function renderResponseLine(line: string, demo: ApiDemoConfig): ReactNode {
  if (!line.trim()) return <Muted> </Muted>

  const segments: ReactNode[] = []
  let cursor = 0
  const matches = [...line.matchAll(PLACEHOLDER_RE)]

  if (matches.length === 0) return tokenize(line)

  matches.forEach((match, idx) => {
    const start = match.index ?? 0
    if (start > cursor) {
      segments.push(
        <span key={`pre-${idx}`}>{tokenize(line.slice(cursor, start))}</span>
      )
    }
    const placeholder = match[0]
    if (placeholder === '<text>') {
      segments.push(
        <Accent key={`ph-${idx}`} accent={demo.accent}>
          {`"${truncateResponse(demo)}"`}
        </Accent>
      )
    } else if (placeholder === '<tokens>') {
      segments.push(<NumberText key={`ph-${idx}`}>{demo.tokens}</NumberText>)
    } else if (placeholder === '<in>') {
      segments.push(
        <NumberText key={`ph-${idx}`}>
          {Math.floor(demo.tokens * 0.4)}
        </NumberText>
      )
    } else if (placeholder === '<out>') {
      segments.push(
        <NumberText key={`ph-${idx}`}>
          {Math.ceil(demo.tokens * 0.6)}
        </NumberText>
      )
    } else {
      segments.push(<Muted key={`ph-${idx}`}>{placeholder}</Muted>)
    }
    cursor = start + placeholder.length
  })

  if (cursor < line.length) {
    segments.push(<span key='tail'>{tokenize(line.slice(cursor))}</span>)
  }

  return segments
}

function truncateResponse(demo: ApiDemoConfig): string {
  const map: Record<string, string> = {
    'gpt-chat': 'Chat request routed.',
    responses: 'Response workflow ready.',
    claude: 'Claude message routed.',
    gemini: 'Gemini request served.',
  }
  return map[demo.id] ?? '...'
}

function tokenize(input: string): ReactNode {
  // Split string into "..." string runs and the rest, then color keys/punct.
  const segments: ReactNode[] = []
  let cursor = 0
  const matches = [...input.matchAll(STRING_RE)]

  matches.forEach((match, idx) => {
    const start = match.index ?? 0
    if (start > cursor) {
      segments.push(
        <Muted key={`m-${idx}`}>{input.slice(cursor, start)}</Muted>
      )
    }
    const text = match[0]
    const after = input.slice(start + text.length).trimStart()
    const isKey = after.startsWith(':')
    if (isKey) {
      segments.push(<Key key={`k-${idx}`}>{text}</Key>)
    } else {
      segments.push(<StringText key={`s-${idx}`}>{text}</StringText>)
    }
    cursor = start + text.length
  })

  if (cursor < input.length) {
    segments.push(<Muted key='tail'>{input.slice(cursor)}</Muted>)
  }

  return segments
}

function CodeLine(props: { children: ReactNode; indent?: number }) {
  return (
    <div className='break-words whitespace-pre-wrap'>
      {props.indent ? (
        <span
          aria-hidden
          className='inline-block'
          style={{ width: `${props.indent}ch` }}
        />
      ) : null}
      {props.children}
    </div>
  )
}

function Command(props: { children: ReactNode }) {
  return <span className='code-command'>{props.children}</span>
}

function Flag(props: { children: ReactNode }) {
  return <span className='code-flag'>{props.children}</span>
}

function Key(props: { children: ReactNode }) {
  return <span className='code-flag'>{props.children}</span>
}

function StringText(props: { children: ReactNode }) {
  return <span className='code-string'>{props.children}</span>
}

function NumberText(props: { children: ReactNode }) {
  return <span className='code-number'>{props.children}</span>
}

function Muted(props: { children: ReactNode }) {
  return <span className='code-muted'>{props.children}</span>
}

function Accent(props: { children: ReactNode; accent: AccentTone }) {
  const tone = ACCENT_CLASSES[props.accent]
  return (
    <span className='font-medium' style={{ color: tone.tone }}>
      {props.children}
    </span>
  )
}
