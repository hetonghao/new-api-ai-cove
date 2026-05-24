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
import { useEffect, useRef, type CSSProperties } from 'react'
import {
  Mesh,
  NoBlending,
  OrthographicCamera,
  PlaneGeometry,
  Scene,
  ShaderMaterial,
  Vector2,
  Vector3,
  WebGLRenderer,
} from 'three'

const vertexShader = `
precision highp float;

void main() {
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`

const fragmentShader = `
precision highp float;

uniform float iTime;
uniform vec3 iResolution;
uniform float animationSpeed;

uniform bool enableTop;
uniform bool enableMiddle;
uniform bool enableBottom;

uniform int topLineCount;
uniform int middleLineCount;
uniform int bottomLineCount;

uniform float topLineDistance;
uniform float middleLineDistance;
uniform float bottomLineDistance;

uniform vec3 topWavePosition;
uniform vec3 middleWavePosition;
uniform vec3 bottomWavePosition;

uniform vec2 iMouse;
uniform bool interactive;
uniform float bendRadius;
uniform float bendStrength;
uniform float bendInfluence;

uniform bool parallax;
uniform vec2 parallaxOffset;

uniform vec3 lineGradient[8];
uniform int lineGradientCount;

const vec3 BLACK = vec3(0.0);
const vec3 PINK = vec3(233.0, 71.0, 245.0) / 255.0;
const vec3 BLUE = vec3(47.0, 75.0, 162.0) / 255.0;

mat2 rotate(float r) {
  return mat2(cos(r), sin(r), -sin(r), cos(r));
}

vec3 background_color(vec2 uv) {
  vec3 col = vec3(0.0);

  float y = sin(uv.x - 0.2) * 0.3 - 0.1;
  float m = uv.y - y;

  col += mix(BLUE, BLACK, smoothstep(0.0, 1.0, abs(m)));
  col += mix(PINK, BLACK, smoothstep(0.0, 1.0, abs(m - 0.8)));
  return col * 0.5;
}

vec3 getLineColor(float t, vec3 baseColor) {
  if (lineGradientCount <= 0) {
    return baseColor;
  }

  vec3 gradientColor;

  if (lineGradientCount == 1) {
    gradientColor = lineGradient[0];
  } else {
    float clampedT = clamp(t, 0.0, 0.9999);
    float scaled = clampedT * float(lineGradientCount - 1);
    int idx = int(floor(scaled));
    float f = fract(scaled);
    int idx2 = min(idx + 1, lineGradientCount - 1);

    vec3 c1 = lineGradient[idx];
    vec3 c2 = lineGradient[idx2];

    gradientColor = mix(c1, c2, f);
  }

  return gradientColor * 0.5;
}

float wave(vec2 uv, float offset, vec2 screenUv, vec2 mouseUv, bool shouldBend) {
  float time = iTime * animationSpeed;

  float x_offset = offset;
  float x_movement = time * 0.1;
  float amp = sin(offset + time * 0.2) * 0.3;
  float y = sin(uv.x + x_offset + x_movement) * amp;

  if (shouldBend) {
    vec2 d = screenUv - mouseUv;
    float influence = exp(-dot(d, d) * bendRadius);
    float bendOffset = (mouseUv.y - screenUv.y) * influence * bendStrength * bendInfluence;
    y += bendOffset;
  }

  float distanceToLine = abs(uv.y - y);
  float core = 0.009 / max(distanceToLine + 0.0035, 1e-3);
  float halo = 0.003 / max(distanceToLine + 0.028, 1e-3);
  return core + halo * 0.26;
}

void mainImage(out vec4 fragColor, in vec2 fragCoord) {
  vec2 baseUv = (2.0 * fragCoord - iResolution.xy) / iResolution.y;
  baseUv.y *= -1.0;

  if (parallax) {
    baseUv += parallaxOffset;
  }

  vec3 col = vec3(0.0);
  vec3 b = lineGradientCount > 0 ? vec3(0.0) : background_color(baseUv);

  vec2 mouseUv = vec2(0.0);
  if (interactive) {
    mouseUv = (2.0 * iMouse - iResolution.xy) / iResolution.y;
    mouseUv.y *= -1.0;
  }

  if (enableBottom) {
    for (int i = 0; i < bottomLineCount; ++i) {
      float fi = float(i);
      float t = fi / max(float(bottomLineCount - 1), 1.0);
      vec3 lineCol = getLineColor(t, b);

      float angle = bottomWavePosition.z * log(length(baseUv) + 1.0);
      vec2 ruv = baseUv * rotate(angle);
      col += lineCol * wave(
        ruv + vec2(bottomLineDistance * fi + bottomWavePosition.x, bottomWavePosition.y),
        1.5 + 0.2 * fi,
        baseUv,
        mouseUv,
        interactive
      ) * 0.2;
    }
  }

  if (enableMiddle) {
    for (int i = 0; i < middleLineCount; ++i) {
      float fi = float(i);
      float t = fi / max(float(middleLineCount - 1), 1.0);
      vec3 lineCol = getLineColor(t, b);

      float angle = middleWavePosition.z * log(length(baseUv) + 1.0);
      vec2 ruv = baseUv * rotate(angle);
      col += lineCol * wave(
        ruv + vec2(middleLineDistance * fi + middleWavePosition.x, middleWavePosition.y),
        2.0 + 0.15 * fi,
        baseUv,
        mouseUv,
        interactive
      );
    }
  }

  if (enableTop) {
    for (int i = 0; i < topLineCount; ++i) {
      float fi = float(i);
      float t = fi / max(float(topLineCount - 1), 1.0);
      vec3 lineCol = getLineColor(t, b);

      float angle = topWavePosition.z * log(length(baseUv) + 1.0);
      vec2 ruv = baseUv * rotate(angle);
      ruv.x *= -1.0;
      col += lineCol * wave(
        ruv + vec2(topLineDistance * fi + topWavePosition.x, topWavePosition.y),
        1.0 + 0.2 * fi,
        baseUv,
        mouseUv,
        interactive
      ) * 0.1;
    }
  }

  fragColor = vec4(col, 1.0);
}

void main() {
  vec4 color = vec4(0.0);
  mainImage(color, gl_FragCoord.xy);

  float lineEnergy = max(max(color.r, color.g), color.b);
  float lineAlpha = smoothstep(0.18, 0.62, lineEnergy);
  vec3 lineColor = clamp(color.rgb / max(lineEnergy, 0.001), 0.0, 1.0);
  gl_FragColor = vec4(lineColor * lineAlpha, lineAlpha);
}
`

const MAX_GRADIENT_STOPS = 8
type WaveType = 'top' | 'middle' | 'bottom'

const DEFAULT_ENABLED_WAVES: WaveType[] = ['top', 'middle', 'bottom']
const DEFAULT_LINE_COUNT = [6]
const DEFAULT_LINE_DISTANCE = [5]
const DEFAULT_BOTTOM_WAVE_POSITION = { x: 2, y: -0.7, rotate: -1 }
const FRAME_INTERVAL_MS = 1000 / 45
const MAX_DELTA_SECONDS = 0.06
const MAX_PIXEL_RATIO = 1.5

type WavePosition = {
  x: number
  y: number
  rotate: number
}

export type FloatingLinesProps = {
  animationSpeed?: number
  bendRadius?: number
  bendStrength?: number
  className?: string
  enabledWaves?: WaveType[]
  interactive?: boolean
  lineCount?: number | number[]
  lineDistance?: number | number[]
  linesGradient?: string[]
  middleWavePosition?: WavePosition
  mixBlendMode?: CSSProperties['mixBlendMode']
  mouseDamping?: number
  parallax?: boolean
  parallaxStrength?: number
  style?: CSSProperties
  topWavePosition?: WavePosition
  bottomWavePosition?: WavePosition
}

function hexToVec3(hex: string): Vector3 {
  let value = hex.trim()

  if (value.startsWith('#')) {
    value = value.slice(1)
  }

  let r = 255
  let g = 255
  let b = 255

  if (value.length === 3) {
    r = parseInt(value[0] + value[0], 16)
    g = parseInt(value[1] + value[1], 16)
    b = parseInt(value[2] + value[2], 16)
  } else if (value.length === 6) {
    r = parseInt(value.slice(0, 2), 16)
    g = parseInt(value.slice(2, 4), 16)
    b = parseInt(value.slice(4, 6), 16)
  }

  return new Vector3(r / 255, g / 255, b / 255)
}

function getWaveSetting(
  value: number | number[],
  enabledWaves: WaveType[],
  waveType: WaveType,
  fallback: number
): number {
  if (typeof value === 'number') return value

  const waveIndex = enabledWaves.indexOf(waveType)
  if (waveIndex === -1) return fallback

  return value[waveIndex] ?? fallback
}

function getWavePosition(
  position: WavePosition | undefined,
  fallback: WavePosition
): Vector3 {
  return new Vector3(
    position?.x ?? fallback.x,
    position?.y ?? fallback.y,
    position?.rotate ?? fallback.rotate
  )
}

export function FloatingLines({
  animationSpeed = 1,
  bendRadius = 5,
  bendStrength = -0.5,
  bottomWavePosition = DEFAULT_BOTTOM_WAVE_POSITION,
  className,
  enabledWaves = DEFAULT_ENABLED_WAVES,
  interactive = true,
  lineCount = DEFAULT_LINE_COUNT,
  lineDistance = DEFAULT_LINE_DISTANCE,
  linesGradient,
  middleWavePosition,
  mixBlendMode = 'screen',
  mouseDamping = 0.05,
  parallax = true,
  parallaxStrength = 0.2,
  style,
  topWavePosition,
}: FloatingLinesProps) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const targetMouseRef = useRef<Vector2>(new Vector2(-1000, -1000))
  const currentMouseRef = useRef<Vector2>(new Vector2(-1000, -1000))
  const targetInfluenceRef = useRef<number>(0)
  const currentInfluenceRef = useRef<number>(0)
  const targetParallaxRef = useRef<Vector2>(new Vector2(0, 0))
  const currentParallaxRef = useRef<Vector2>(new Vector2(0, 0))

  const topEnabled = enabledWaves.includes('top')
  const middleEnabled = enabledWaves.includes('middle')
  const bottomEnabled = enabledWaves.includes('bottom')

  const topLineCount = topEnabled
    ? getWaveSetting(lineCount, enabledWaves, 'top', 6)
    : 0
  const middleLineCount = middleEnabled
    ? getWaveSetting(lineCount, enabledWaves, 'middle', 6)
    : 0
  const bottomLineCount = bottomEnabled
    ? getWaveSetting(lineCount, enabledWaves, 'bottom', 6)
    : 0

  const topLineDistance = topEnabled
    ? getWaveSetting(lineDistance, enabledWaves, 'top', 0.1) * 0.01
    : 0.01
  const middleLineDistance = middleEnabled
    ? getWaveSetting(lineDistance, enabledWaves, 'middle', 0.1) * 0.01
    : 0.01
  const bottomLineDistance = bottomEnabled
    ? getWaveSetting(lineDistance, enabledWaves, 'bottom', 0.1) * 0.01
    : 0.01

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return

    let active = true
    let elapsedTime = 0
    let inViewport = true
    let lastRenderTime = 0
    let pageVisible = document.visibilityState !== 'hidden'

    const scene = new Scene()
    const camera = new OrthographicCamera(-1, 1, 1, -1, 0, 1)
    camera.position.z = 1

    const renderer = new WebGLRenderer({
      alpha: true,
      antialias: false,
      premultipliedAlpha: true,
      powerPreference: 'high-performance',
    })
    renderer.setClearAlpha(0)
    renderer.setPixelRatio(
      Math.min(window.devicePixelRatio || 1, MAX_PIXEL_RATIO)
    )
    renderer.domElement.style.width = '100%'
    renderer.domElement.style.height = '100%'
    renderer.domElement.style.display = 'block'
    container.appendChild(renderer.domElement)

    const uniforms = {
      animationSpeed: { value: animationSpeed },
      bendInfluence: { value: 0 },
      bendRadius: { value: bendRadius },
      bendStrength: { value: bendStrength },
      bottomLineCount: { value: bottomLineCount },
      bottomLineDistance: { value: bottomLineDistance },
      bottomWavePosition: {
        value: getWavePosition(
          bottomWavePosition,
          DEFAULT_BOTTOM_WAVE_POSITION
        ),
      },
      enableBottom: { value: bottomEnabled },
      enableMiddle: { value: middleEnabled },
      enableTop: { value: topEnabled },
      interactive: { value: interactive },
      iMouse: { value: new Vector2(-1000, -1000) },
      iResolution: { value: new Vector3(1, 1, 1) },
      iTime: { value: 0 },
      lineGradient: {
        value: Array.from(
          { length: MAX_GRADIENT_STOPS },
          () => new Vector3(1, 1, 1)
        ),
      },
      lineGradientCount: { value: 0 },
      middleLineCount: { value: middleLineCount },
      middleLineDistance: { value: middleLineDistance },
      middleWavePosition: {
        value: getWavePosition(middleWavePosition, {
          x: 5,
          y: 0,
          rotate: 0.2,
        }),
      },
      parallax: { value: parallax },
      parallaxOffset: { value: new Vector2(0, 0) },
      topLineCount: { value: topLineCount },
      topLineDistance: { value: topLineDistance },
      topWavePosition: {
        value: getWavePosition(topWavePosition, {
          x: 10,
          y: 0.5,
          rotate: -0.4,
        }),
      },
    }

    if (linesGradient && linesGradient.length > 0) {
      const stops = linesGradient.slice(0, MAX_GRADIENT_STOPS)
      uniforms.lineGradientCount.value = stops.length

      stops.forEach((hex, index) => {
        const color = hexToVec3(hex)
        uniforms.lineGradient.value[index].set(color.x, color.y, color.z)
      })
    }

    const material = new ShaderMaterial({
      blending: NoBlending,
      fragmentShader,
      transparent: true,
      uniforms,
      vertexShader,
    })
    const geometry = new PlaneGeometry(2, 2)
    const mesh = new Mesh(geometry, material)

    scene.add(mesh)

    const setSize = () => {
      if (!active) return
      const width = container.clientWidth || 1
      const height = container.clientHeight || 1

      renderer.setSize(width, height, false)
      uniforms.iResolution.value.set(
        renderer.domElement.width,
        renderer.domElement.height,
        1
      )
    }

    const resizeObserver =
      typeof ResizeObserver !== 'undefined' ? new ResizeObserver(setSize) : null
    const handlePointerMove = (event: PointerEvent) => {
      const rect = renderer.domElement.getBoundingClientRect()
      const x = event.clientX - rect.left
      const y = event.clientY - rect.top
      const dpr = renderer.getPixelRatio()

      targetMouseRef.current.set(x * dpr, (rect.height - y) * dpr)
      targetInfluenceRef.current = 1

      if (parallax) {
        const offsetX = (x - rect.width / 2) / rect.width
        const offsetY = -(y - rect.height / 2) / rect.height
        targetParallaxRef.current.set(
          offsetX * parallaxStrength,
          offsetY * parallaxStrength
        )
      }
    }

    const handlePointerLeave = () => {
      targetInfluenceRef.current = 0
    }

    if (interactive) {
      renderer.domElement.addEventListener('pointermove', handlePointerMove, {
        passive: true,
      })
      renderer.domElement.addEventListener('pointerleave', handlePointerLeave)
    }

    let animationFrame = 0
    let loopRunning = false

    const stopRenderLoop = () => {
      if (!loopRunning) return

      loopRunning = false
      cancelAnimationFrame(animationFrame)
      animationFrame = 0
      lastRenderTime = 0
    }

    const renderLoop = (frameTime = 0) => {
      if (!active) return

      if (!inViewport || !pageVisible) {
        stopRenderLoop()
        return
      }

      animationFrame = requestAnimationFrame(renderLoop)

      const frameDelta = frameTime - lastRenderTime
      if (lastRenderTime > 0 && frameDelta < FRAME_INTERVAL_MS) return

      elapsedTime +=
        lastRenderTime === 0
          ? 0
          : Math.min(frameDelta / 1000, MAX_DELTA_SECONDS)
      lastRenderTime = frameTime
      uniforms.iTime.value = elapsedTime

      if (interactive) {
        currentMouseRef.current.lerp(targetMouseRef.current, mouseDamping)
        uniforms.iMouse.value.copy(currentMouseRef.current)
        currentInfluenceRef.current +=
          (targetInfluenceRef.current - currentInfluenceRef.current) *
          mouseDamping
        uniforms.bendInfluence.value = currentInfluenceRef.current
      }

      if (parallax) {
        currentParallaxRef.current.lerp(targetParallaxRef.current, mouseDamping)
        uniforms.parallaxOffset.value.copy(currentParallaxRef.current)
      }

      renderer.render(scene, camera)
    }

    const startRenderLoop = () => {
      if (loopRunning || !active || !inViewport || !pageVisible) return

      loopRunning = true
      lastRenderTime = 0
      animationFrame = requestAnimationFrame(renderLoop)
    }

    const updateRenderLoop = () => {
      if (inViewport && pageVisible) {
        startRenderLoop()
      } else {
        stopRenderLoop()
      }
    }

    const intersectionObserver =
      typeof IntersectionObserver !== 'undefined'
        ? new IntersectionObserver(
            ([entry]) => {
              inViewport = entry?.isIntersecting ?? true
              updateRenderLoop()
            },
            { rootMargin: '240px' }
          )
        : null

    const handleVisibilityChange = () => {
      pageVisible = document.visibilityState !== 'hidden'
      updateRenderLoop()
    }

    setSize()
    resizeObserver?.observe(container)
    intersectionObserver?.observe(container)
    document.addEventListener('visibilitychange', handleVisibilityChange)
    updateRenderLoop()

    return () => {
      active = false
      stopRenderLoop()
      resizeObserver?.disconnect()
      intersectionObserver?.disconnect()
      document.removeEventListener('visibilitychange', handleVisibilityChange)

      if (interactive) {
        renderer.domElement.removeEventListener(
          'pointermove',
          handlePointerMove
        )
        renderer.domElement.removeEventListener(
          'pointerleave',
          handlePointerLeave
        )
      }

      scene.remove(mesh)
      geometry.dispose()
      material.dispose()
      renderer.dispose()
      renderer.forceContextLoss()
      renderer.domElement.remove()
    }
  }, [
    animationSpeed,
    bendRadius,
    bendStrength,
    bottomEnabled,
    bottomLineCount,
    bottomLineDistance,
    bottomWavePosition,
    enabledWaves,
    interactive,
    lineCount,
    lineDistance,
    linesGradient,
    middleEnabled,
    middleLineCount,
    middleLineDistance,
    middleWavePosition,
    mouseDamping,
    parallax,
    parallaxStrength,
    topEnabled,
    topLineCount,
    topLineDistance,
    topWavePosition,
  ])

  return (
    <div
      aria-hidden='true'
      className={className}
      ref={containerRef}
      style={{
        mixBlendMode,
        ...style,
      }}
    />
  )
}
