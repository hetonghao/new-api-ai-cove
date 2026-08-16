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
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'

function readSource(relativePath) {
  return readFileSync(path.join(process.cwd(), relativePath), 'utf8')
}

function readLocale(locale) {
  return JSON.parse(readSource(`src/i18n/locales/${locale}.json`)).translation
}

function visibleCopy(value) {
  return value.replaceAll('\u2060', '').replaceAll('\u00a0', ' ')
}

const authLayout = readSource('src/features/auth/auth-layout.tsx')
const cta = readSource('src/features/home/components/sections/cta.tsx')
const features = readSource(
  'src/features/home/components/sections/features.tsx'
)
const hero = readSource('src/features/home/components/sections/hero.tsx')
const howItWorks = readSource(
  'src/features/home/components/sections/how-it-works.tsx'
)
const signIn = readSource('src/features/auth/sign-in/index.tsx')
const signUp = readSource('src/features/auth/sign-up/index.tsx')
const styles = readSource('src/styles/index.css')
const en = readLocale('en')
const zh = readLocale('zh')

test('Chinese homepage copy renders a localized three-step story', () => {
  assert.equal(zh['Stable access'], '稳定接入')
  assert.equal(
    zh[
      'Keep local tools and CI on the same compatible API contract without changing your client workflow.'
    ],
    '让本地工具和 CI 始终使用同\u2060一\u2060套\u2060兼\u2060容\u00a0API，无需改动现\u2060有\u2060客\u2060户\u2060端\u2060工\u2060作\u2060流。'
  )
  assert.equal(
    visibleCopy(
      zh[
        'Keep local tools and CI on the same compatible API contract without changing your client workflow.'
      ]
    ),
    '让本地工具和 CI 始终使用同一套兼容 API，无需改动现有客户端工作流。'
  )
  assert.equal(
    zh[
      'One compatible endpoint keeps local assistants, official CLIs, and coding agents aligned.'
    ],
    '本地助手、CLI、编程代理统一接入。'
  )
  assert.equal(
    zh['Four reasons developers ship faster on AI Cove.'],
    'AI Cove 加速开发者交付的四个理由。'
  )

  assert.match(features, /t\('Stable access'\)/)
  assert.match(
    howItWorks,
    /t\('Step \{\{number\}\}',\s*\{\s*number:\s*step\.num\s*\}\)/
  )
  assert.equal(zh['Step {{number}}'].replace('{{number}}', '01'), '步骤 01')

  const ctaKey =
    'Sign up and create a key, check pricing, enter the Base URL: get your first 200 OK in three steps.'
  assert.match(
    cta,
    new RegExp(ctaKey.replaceAll(/[.*+?^${}()|[\]\\]/g, '\\$&'))
  )
  assert.equal(en[ctaKey], ctaKey)
  assert.equal(zh[ctaKey], '注册后建 Key、查价、填 URL，三步完成首次调用')
})

test('Chinese homepage uses concise copy for narrow sections', () => {
  assert.equal(zh['Ready to strengthen'], '让 AI 应用')
  assert.equal(zh['your AI application?'], '更进一步')
  assert.equal(
    zh[
      'One compatible endpoint keeps local assistants, official CLIs, and coding agents aligned.'
    ],
    '本地助手、CLI、编程代理统一接入。'
  )
  assert.equal(
    zh[
      'Open-source local AI assistant that can execute tasks on your computer through chat, not just respond.'
    ],
    '开源本地 AI 助手，能直接操作电脑。'
  )
  assert.equal(
    zh[
      'Anthropic official CLI with Extended Thinking support for code work that needs deeper reasoning.'
    ],
    'Anthropic 官方 CLI，原生支持 Extended Thinking，适合复杂编码。'
  )
  assert.equal(
    zh[
      'OpenAI coding agent for larger refactors, bug fixes, and tests across real repositories.'
    ],
    'OpenAI 编程代理，可重构、修 Bug、写测试，长任务稳定。'
  )
  assert.equal(
    zh[
      'Google open-source terminal agent for coding, debugging, and workflow automation from the command line.'
    ],
    'Google 开源终端 AI 代理，可用 Gemini 编码、调试并自动化流程。'
  )
  assert.equal(
    zh[
      'Connect through OpenAI, Claude, Gemini, and other compatible API routes'
    ],
    'OpenAI、Claude、Gemini 等兼容 API'
  )

  assert.match(cta, /t\('Ready to strengthen'\)/)
  assert.match(
    features,
    /t\(\s*'One compatible endpoint keeps local assistants/
  )
  assert.match(features, /t\(\s*'Open-source local AI assistant/)
  assert.match(howItWorks, /t\(\s*'Connect through OpenAI, Claude, Gemini/)
})

test('Chinese homepage protects semantic phrases from awkward wrapping', () => {
  const valueKey =
    'Fast direct access, no account-ban risk, non-expiring balance, and better value. Low-latency access to ChatGPT, Claude, Gemini, and other frontier models.'

  assert.equal(zh['frontier AI models'], '顶尖\u2004AI 模型')
  assert.equal(zh['Better value'], '高性价比')
  assert.match(en[valueKey], /<nowrap>better value<\/nowrap>/)
  assert.match(zh[valueKey], /<nowrap>高性价比<\/nowrap>/)
  assert.match(
    zh[valueKey],
    /<nowrap>ChatGPT、Claude、Gemini 等顶尖模型<\/nowrap>。/
  )
  assert.doesNotMatch(zh[valueKey], /等<nowrap>顶尖模型<\/nowrap>/)
  assert.doesNotMatch(hero, /home-title-accent home-cjk-nowrap/)
  assert.match(hero, /nowrap:\s*<span className='home-cjk-nowrap'\s*\/>/)
  assert.match(authLayout, /label:\s*'Better value'/)
  assert.match(authLayout, /\{t\(label\)\}/)
  assert.match(styles, /\.home-cjk-nowrap\s*\{\s*white-space:\s*nowrap;/)
  assert.doesNotMatch(
    hero,
    /\{oneApiSitePrefix\}\s*\{' '\}\s*<span className='home-title-api'>/
  )
})

test('homepage ecosystem uses the login icons in the requested order', () => {
  assert.match(
    features,
    /import CodexIcon from '@lobehub\/icons\/es\/Codex\/components\/Color'/
  )
  assert.match(
    features,
    /import ClaudeCodeIcon from '@lobehub\/icons\/es\/ClaudeCode\/components\/Color'/
  )
  assert.match(
    features,
    /import GeminiIcon from '@lobehub\/icons\/es\/Gemini\/components\/Color'/
  )
  assert.match(
    features,
    /import OpenClawIcon from '@lobehub\/icons\/es\/OpenClaw\/components\/Color'/
  )
  assert.ok(
    features.indexOf("title: 'Codex'") < features.indexOf("title: 'OpenClaw'")
  )
  assert.match(features, /title: 'Codex',[\s\S]*?icon: CodexIcon/)
  assert.match(features, /title: 'Claude Code',[\s\S]*?icon: ClaudeCodeIcon/)
  assert.match(features, /title: 'OpenClaw',[\s\S]*?icon: OpenClawIcon/)
  assert.match(features, /title: 'Gemini CLI',[\s\S]*?icon: GeminiIcon/)
  assert.match(features, /<h4 className='eco-tool-title'>/)
  assert.doesNotMatch(features, /className='eco-symbol'/)
  assert.doesNotMatch(styles, /\.eco-symbol/)
})

test('Chinese proof copy keeps visible text while protecting semantic units', () => {
  const headingKey =
    'Unlock your coding throughput and let frontier AI write with you'
  const compatibilityKey =
    'Preserve common model behaviors across Chat, Responses, Claude, Gemini, and image workflows.'
  const availabilityKey =
    'Health checks, fallback order, and grouped channels keep critical requests moving.'
  const integrationKey =
    'Change the Base URL, keep your client code, and route through OpenAI-compatible APIs.'

  assert.equal(
    zh[headingKey],
    '释放你的编\u2060程\u2060潜\u2060能，让\u2060顶\u2060尖\u00a0AI 为\u2060你\u2060写\u2060代\u2060码'
  )
  assert.equal(
    visibleCopy(zh[headingKey]),
    '释放你的编程潜能，让顶尖 AI 为你写代码'
  )
  assert.equal(
    zh[compatibilityKey],
    '一次接入即兼容官方 API 行为，保\u2060留\u2060主\u2060流\u2060模\u2060型\u2060常\u2060用\u2060能\u2060力。'
  )
  assert.equal(
    visibleCopy(zh[compatibilityKey]),
    '一次接入即兼容官方 API 行为，保留主流模型常用能力。'
  )
  assert.equal(
    zh[availabilityKey],
    '分布式架构设计与自动故障转移，关\u2060键\u2060时\u2060刻依\u2060然\u2060可\u2060用。'
  )
  assert.equal(
    visibleCopy(zh[availabilityKey]),
    '分布式架构设计与自动故障转移，关键时刻依然可用。'
  )
  assert.equal(
    zh[integrationKey],
    '只需修改 API 地址即可使用，无\u2060需\u2060重\u2060写\u2060现\u2060有\u2060业\u2060务\u2060逻\u2060辑。'
  )
  assert.equal(
    visibleCopy(zh[integrationKey]),
    '只需修改 API 地址即可使用，无需重写现有业务逻辑。'
  )
})

test('login prompts do not append an English period after localized links', () => {
  assert.doesNotMatch(signIn, /<\/Link>\s*\.\s*<\/p>/)
  assert.doesNotMatch(signUp, /<\/Link>\s*\.\s*<\/p>/)
})
