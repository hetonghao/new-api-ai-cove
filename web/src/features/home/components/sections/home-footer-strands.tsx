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
import { useEffect, useRef, type ReactElement } from 'react'

// Adapted from AI Cove Turbo's React Bits Strands port. The upstream source
// is pinned at 1320d40a8318ac7d4fe6690c7206ceda8cdd59bd.
const COLOR_COUNT = 3
const STRAND_COUNT = 3
const VERTEX_SHADER = `#version 300 es
in vec2 position;
void main() {
  gl_Position = vec4(position, 0.0, 1.0);
}
`
const FRAGMENT_SHADER = `#version 300 es
precision highp float;

uniform float uTime;
uniform vec2 uResolution;
uniform vec3 uColors[${COLOR_COUNT}];

out vec4 fragColor;

const float PI = 3.14159265;
const float VISUAL_SCALE = 0.5;

vec3 samplePalette(float t) {
  float scaled = fract(t) * float(${COLOR_COUNT});
  int index = int(floor(scaled));
  int nextIndex = index + 1;
  if (nextIndex >= ${COLOR_COUNT}) nextIndex = 0;
  return mix(uColors[index], uColors[nextIndex], fract(scaled));
}

void main() {
  vec2 uv = (gl_FragCoord.xy - 0.5 * uResolution) / (uResolution.x / 1.9);
  uv /= 1.875;
  uv /= VISUAL_SCALE;

  float energy = 0.436;
  float envelope = pow(max(cos(uv.x * PI * 1.3), 0.0), 3.0);
  vec3 color = vec3(0.0);

  for (int i = 0; i < ${STRAND_COUNT}; i++) {
    float strand = float(i);
    float phase = strand * 1.7;
    float frequency = 2.0 + strand * 0.35;
    float velocity = 1.4 + strand * 1.2;
    float time = uTime * 0.5;
    float wave = sin(uv.x * frequency + time * velocity + phase) * 0.60
               + sin(uv.x * frequency * 1.1 - time * velocity * 0.7 + phase * 1.7) * 0.40;
    float amplitude = (0.1 + 0.02 * energy) * envelope;
    float distanceToStrand = abs(uv.y - wave * amplitude);
    float thickness = (0.001 + 0.05 * energy) * (0.35 + envelope) * 0.7;
    float light = thickness / (distanceToStrand + thickness * 0.45);
    light = pow(light, 2.4);

    float hue = strand / float(${STRAND_COUNT}) + uv.x * 0.30 + uTime * 0.04;
    color += samplePalette(hue) * light * envelope;
  }

  color *= 0.45 + 0.7 * energy;
  color = 1.0 - exp(-color * 2.6);
  float gray = dot(color, vec3(0.2126, 0.7152, 0.0722));
  color = max(mix(vec3(gray), color, 2.0), 0.0);
  float intensity = clamp(max(max(color.r, color.g), color.b), 0.0, 1.0);
  float fade = smoothstep(0.035, 0.14, intensity);
  fragColor = vec4(color * fade, intensity * fade);
}
`

const COLOR_TOKENS = [
  '--home-turbo-orange',
  '--home-turbo-violet',
  '--home-turbo-cyan',
] as const

function parseColor(value: string): readonly [number, number, number] {
  const normalized = value.trim().replace('#', '')
  const color = Number.parseInt(normalized, 16)
  if (normalized.length !== 6 || !Number.isFinite(color)) return [1, 1, 1]
  return [
    ((color >> 16) & 255) / 255,
    ((color >> 8) & 255) / 255,
    (color & 255) / 255,
  ]
}

export function HomeFooterStrands(): ReactElement {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    const gl = canvas.getContext('webgl2', {
      alpha: true,
      antialias: true,
      premultipliedAlpha: true,
    })
    if (!gl) return

    const compile = (type: number, source: string): WebGLShader | null => {
      const shader = gl.createShader(type)
      if (!shader) return null
      gl.shaderSource(shader, source)
      gl.compileShader(shader)
      if (gl.getShaderParameter(shader, gl.COMPILE_STATUS)) return shader
      gl.deleteShader(shader)
      return null
    }
    const vertex = compile(gl.VERTEX_SHADER, VERTEX_SHADER)
    const fragment = compile(gl.FRAGMENT_SHADER, FRAGMENT_SHADER)
    const program = gl.createProgram()
    if (!vertex || !fragment || !program) {
      if (vertex) gl.deleteShader(vertex)
      if (fragment) gl.deleteShader(fragment)
      if (program) gl.deleteProgram(program)
      return
    }

    gl.attachShader(program, vertex)
    gl.attachShader(program, fragment)
    gl.linkProgram(program)
    gl.deleteShader(vertex)
    gl.deleteShader(fragment)
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      gl.deleteProgram(program)
      return
    }

    const buffer = gl.createBuffer()
    if (!buffer) {
      gl.deleteProgram(program)
      return
    }
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer)
    gl.bufferData(
      gl.ARRAY_BUFFER,
      new Float32Array([-1, -1, 3, -1, -1, 3]),
      gl.STATIC_DRAW
    )
    const position = gl.getAttribLocation(program, 'position')
    gl.enableVertexAttribArray(position)
    gl.vertexAttribPointer(position, 2, gl.FLOAT, false, 0, 0)

    const styles = window.getComputedStyle(canvas)
    const palette = COLOR_TOKENS.flatMap((name) =>
      parseColor(styles.getPropertyValue(name))
    )
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)')
    const time = gl.getUniformLocation(program, 'uTime')
    const resolution = gl.getUniformLocation(program, 'uResolution')
    let frame = 0
    let sized = false

    gl.clearColor(0, 0, 0, 0)
    gl.enable(gl.BLEND)
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
    gl.useProgram(program)
    gl.uniform3fv(
      gl.getUniformLocation(program, 'uColors[0]'),
      new Float32Array(palette)
    )

    const draw = (timestamp = 0): void => {
      if (!sized) return
      gl.clear(gl.COLOR_BUFFER_BIT)
      gl.uniform1f(time, timestamp * 0.001)
      gl.drawArrays(gl.TRIANGLES, 0, 3)
    }
    const stop = (): void => {
      if (frame) window.cancelAnimationFrame(frame)
      frame = 0
    }
    const animate = (timestamp: number): void => {
      if (document.hidden) {
        frame = 0
        return
      }
      draw(timestamp)
      frame = window.requestAnimationFrame(animate)
    }
    const restart = (): void => {
      stop()
      if (!sized || document.hidden) return
      draw()
      if (!reducedMotion.matches) frame = window.requestAnimationFrame(animate)
    }
    const resize = (): void => {
      const bounds = canvas.getBoundingClientRect()
      sized = Boolean(bounds.width && bounds.height)
      if (!sized) {
        stop()
        return
      }
      const dpr = Math.min(window.devicePixelRatio || 1, 2)
      canvas.width = Math.round(bounds.width * dpr)
      canvas.height = Math.round(bounds.height * dpr)
      gl.viewport(0, 0, canvas.width, canvas.height)
      gl.uniform2f(resolution, canvas.width, canvas.height)
      restart()
    }
    const observer = new ResizeObserver(resize)
    observer.observe(canvas)
    reducedMotion.addEventListener('change', restart)
    document.addEventListener('visibilitychange', restart)
    resize()

    return () => {
      stop()
      observer.disconnect()
      reducedMotion.removeEventListener('change', restart)
      document.removeEventListener('visibilitychange', restart)
      gl.deleteBuffer(buffer)
      gl.deleteProgram(program)
    }
  }, [])

  return (
    <canvas
      ref={canvasRef}
      className='home-footer-strands'
      aria-hidden='true'
    />
  )
}
