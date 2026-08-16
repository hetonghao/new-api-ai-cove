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
import { after, afterEach, beforeEach, describe, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'self',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLCanvasElement',
  'Node',
  'Element',
  'Event',
  'DOMRect',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

class IdleResizeObserver {
  constructor(_callback: ResizeObserverCallback) {}

  observe(): void {}

  unobserve(): void {}

  disconnect(): void {}
}

Object.defineProperty(globalThis, 'ResizeObserver', {
  configurable: true,
  value: IdleResizeObserver,
})

const { cleanup, render } = await import('@testing-library/react')
const { HomeFooterStrands } = await import('../home-footer-strands')

type FakeGlCounters = {
  readonly draws: number
  readonly deletedBuffers: number
  readonly deletedPrograms: number
}

function installFakeWebGl(): () => FakeGlCounters {
  let draws = 0
  let deletedBuffers = 0
  let deletedPrograms = 0
  const shader = {}
  const program = {}
  const buffer = {}
  const uniform = {}
  const gl = {
    VERTEX_SHADER: 1,
    FRAGMENT_SHADER: 2,
    COMPILE_STATUS: 3,
    LINK_STATUS: 4,
    ARRAY_BUFFER: 5,
    STATIC_DRAW: 6,
    FLOAT: 7,
    BLEND: 8,
    ONE: 9,
    ONE_MINUS_SRC_ALPHA: 10,
    COLOR_BUFFER_BIT: 11,
    TRIANGLES: 12,
    createShader: () => shader,
    shaderSource: () => undefined,
    compileShader: () => undefined,
    getShaderParameter: () => true,
    deleteShader: () => undefined,
    createProgram: () => program,
    attachShader: () => undefined,
    linkProgram: () => undefined,
    getProgramParameter: () => true,
    deleteProgram: () => {
      deletedPrograms += 1
    },
    createBuffer: () => buffer,
    bindBuffer: () => undefined,
    bufferData: () => undefined,
    getAttribLocation: () => 0,
    enableVertexAttribArray: () => undefined,
    vertexAttribPointer: () => undefined,
    getUniformLocation: () => uniform,
    clearColor: () => undefined,
    enable: () => undefined,
    blendFunc: () => undefined,
    useProgram: () => undefined,
    uniform3fv: () => undefined,
    clear: () => undefined,
    uniform1f: () => undefined,
    drawArrays: () => {
      draws += 1
    },
    viewport: () => undefined,
    uniform2f: () => undefined,
    deleteBuffer: () => {
      deletedBuffers += 1
    },
  }

  Object.defineProperty(domWindow.HTMLCanvasElement.prototype, 'getContext', {
    configurable: true,
    value: () => gl,
  })

  return () => ({ draws, deletedBuffers, deletedPrograms })
}

function setReducedMotion(matches: boolean): void {
  Object.defineProperty(domWindow, 'matchMedia', {
    configurable: true,
    value: () => ({
      matches,
      media: '(prefers-reduced-motion: reduce)',
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => true,
    }),
  })
}

function setDocumentHidden(hidden: boolean): void {
  Object.defineProperty(document, 'hidden', {
    configurable: true,
    value: hidden,
  })
}

describe('home footer Strands lifecycle', () => {
  beforeEach(() => {
    Object.defineProperty(
      domWindow.HTMLCanvasElement.prototype,
      'getBoundingClientRect',
      {
        configurable: true,
        value: () => new DOMRect(0, 0, 470, 181),
      }
    )
  })

  afterEach(() => cleanup())

  after(() => domWindow.close())

  test('draws one static frame and releases resources for reduced motion', () => {
    const counters = installFakeWebGl()
    let scheduledFrames = 0
    Object.defineProperty(domWindow, 'requestAnimationFrame', {
      configurable: true,
      value: () => {
        scheduledFrames += 1
        return scheduledFrames
      },
    })
    setDocumentHidden(false)
    setReducedMotion(true)

    const rendered = render(<HomeFooterStrands />)

    assert.equal(counters().draws, 1)
    assert.equal(scheduledFrames, 0)
    rendered.unmount()
    assert.equal(counters().deletedBuffers, 1)
    assert.equal(counters().deletedPrograms, 1)
  })

  test('stops while hidden and resumes when the page becomes visible', () => {
    const counters = installFakeWebGl()
    let scheduledFrames = 0
    let cancelledFrames = 0
    Object.defineProperty(domWindow, 'requestAnimationFrame', {
      configurable: true,
      value: () => {
        scheduledFrames += 1
        return scheduledFrames
      },
    })
    Object.defineProperty(domWindow, 'cancelAnimationFrame', {
      configurable: true,
      value: () => {
        cancelledFrames += 1
      },
    })
    setReducedMotion(false)
    setDocumentHidden(true)

    render(<HomeFooterStrands />)
    assert.equal(counters().draws, 0)
    assert.equal(scheduledFrames, 0)

    setDocumentHidden(false)
    document.dispatchEvent(new Event('visibilitychange'))
    assert.equal(counters().draws, 1)
    assert.equal(scheduledFrames, 1)

    setDocumentHidden(true)
    document.dispatchEvent(new Event('visibilitychange'))
    assert.equal(cancelledFrames, 1)
  })
})
