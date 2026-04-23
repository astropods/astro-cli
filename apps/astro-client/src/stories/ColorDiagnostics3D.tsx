import React, { useRef, useState, useMemo } from "react";
import { Canvas, useFrame } from "@react-three/fiber";
import { OrbitControls, Text, Line, Billboard } from "@react-three/drei";
import * as THREE from "three";
import type { ColorEntry } from "./ColorDiagnostics.stories";

const CYLINDER_RADIUS = 1.5;
const CYLINDER_HEIGHT = 2;

/** Convert HSL (h: 0-360, s: 0-1, l: 0-1) to RGB (0-1). */
function hslToRgb(h: number, s: number, l: number): [number, number, number] {
  h /= 360;
  const hue2rgb = (p: number, q: number, t: number) => {
    if (t < 0) t += 1; if (t > 1) t -= 1;
    if (t < 1 / 6) return p + (q - p) * 6 * t;
    if (t < 1 / 2) return q;
    if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6;
    return p;
  };
  if (s === 0) return [l, l, l];
  const q = l < 0.5 ? l * (1 + s) : l + s - l * s;
  const p = 2 * l - q;
  return [hue2rgb(p, q, h + 1 / 3), hue2rgb(p, q, h), hue2rgb(p, q, h - 1 / 3)];
}

/** Convert HSL to a position in the 3D cylinder. */
function hslToPosition(h: number, s: number, l: number): [number, number, number] {
  const angle = (h * Math.PI) / 180;
  const r = s * CYLINDER_RADIUS;
  const x = r * Math.cos(angle);
  const z = r * Math.sin(angle);
  const y = (l - 0.5) * CYLINDER_HEIGHT;
  return [x, y, z];
}

/** A translucent HSL-colored cylinder volume for spatial reference. */
function HslVolume() {
  const geometry = useMemo(() => {
    const hueSteps = 128;
    const lightSteps = 64;
    const geo = new THREE.BufferGeometry();
    const vertices: number[] = [];
    const colors: number[] = [];

    // Helper: get linear-space RGB from HSL using Three.js color management
    const tmpColor = new THREE.Color();
    const hslColor = (h: number, s: number, l: number): [number, number, number] => {
      tmpColor.setHSL(h / 360, s, l, THREE.SRGBColorSpace);
      return [tmpColor.r, tmpColor.g, tmpColor.b];
    };

    // Cylinder side wall
    for (let li = 0; li < lightSteps; li++) {
      const l0 = li / lightSteps;
      const l1 = (li + 1) / lightSteps;
      for (let hi = 0; hi < hueSteps; hi++) {
        const h0 = (hi / hueSteps) * 360;
        const h1 = ((hi + 1) / hueSteps) * 360;

        const positions = [
          hslToPosition(h0, 1, l0), hslToPosition(h1, 1, l0), hslToPosition(h1, 1, l1),
          hslToPosition(h0, 1, l0), hslToPosition(h1, 1, l1), hslToPosition(h0, 1, l1),
        ];
        const wallColors = [
          hslColor(h0, 1, l0), hslColor(h1, 1, l0), hslColor(h1, 1, l1),
          hslColor(h0, 1, l0), hslColor(h1, 1, l1), hslColor(h0, 1, l1),
        ];

        for (const p of positions) vertices.push(...p);
        for (const c of wallColors) colors.push(...c);
      }
    }

    // Cross-section disc at L=0.5 (most vivid colors in HSL)
    const satSteps = 32;
    for (let hi = 0; hi < hueSteps; hi++) {
      const h0 = (hi / hueSteps) * 360;
      const h1 = ((hi + 1) / hueSteps) * 360;
      for (let si = 0; si < satSteps; si++) {
        const s0 = si / satSteps;
        const s1 = (si + 1) / satSteps;

        const positions = [
          hslToPosition(h0, s0, 0.5), hslToPosition(h1, s0, 0.5), hslToPosition(h1, s1, 0.5),
          hslToPosition(h0, s0, 0.5), hslToPosition(h1, s1, 0.5), hslToPosition(h0, s1, 0.5),
        ];
        const discColors = [
          hslColor(h0, s0, 0.5), hslColor(h1, s0, 0.5), hslColor(h1, s1, 0.5),
          hslColor(h0, s0, 0.5), hslColor(h1, s1, 0.5), hslColor(h0, s1, 0.5),
        ];

        for (const p of positions) vertices.push(...p);
        for (const c of discColors) colors.push(...c);
      }
    }

    geo.setAttribute("position", new THREE.Float32BufferAttribute(vertices, 3));
    geo.setAttribute("color", new THREE.Float32BufferAttribute(colors, 3));
    return geo;
  }, []);

  return (
    <mesh geometry={geometry}>
      <meshBasicMaterial vertexColors side={THREE.DoubleSide} />
    </mesh>
  );
}

/** Wireframe ring at a given lightness level. */
function WireRing({ lightness, opacity = 0.15 }: { lightness: number; opacity?: number }) {
  const points: THREE.Vector3[] = [];
  const steps = 64;
  for (let i = 0; i <= steps; i++) {
    const [x, y, z] = hslToPosition((i / steps) * 360, 1, lightness);
    points.push(new THREE.Vector3(x, y, z));
  }
  return <Line points={points} color="#888" lineWidth={0.5} opacity={opacity} transparent />;
}

/** Vertical wireframe strut at a given hue. */
function Strut({ hue }: { hue: number }) {
  const bot = hslToPosition(hue, 1, 0);
  const top = hslToPosition(hue, 1, 1);
  return (
    <Line
      points={[new THREE.Vector3(...bot), new THREE.Vector3(...top)]}
      color="#888" lineWidth={0.5} opacity={0.08} transparent
    />
  );
}

/** A single color data point with hover interaction. */
function ColorPoint({ entry }: { entry: ColorEntry }) {
  const meshRef = useRef<THREE.Mesh>(null);
  const [hovered, setHovered] = useState(false);
  const pos = hslToPosition(entry.h, entry.s, entry.l);
  const targetScale = hovered ? 2 : 1;

  useFrame(() => {
    if (!meshRef.current) return;
    const s = meshRef.current.scale.x;
    const next = THREE.MathUtils.lerp(s, targetScale, 0.15);
    meshRef.current.scale.setScalar(next);
  });

  return (
    <group position={pos}>
      {/* Drop line to the bottom ring */}
      <Line
        points={[new THREE.Vector3(0, 0, 0), new THREE.Vector3(0, -(entry.l - 0) * CYLINDER_HEIGHT + (0.5 - entry.l) * CYLINDER_HEIGHT, 0)]}
        color={entry.hex} lineWidth={0.5} opacity={0.2} transparent
        dashed dashSize={0.03} gapSize={0.03}
      />
      {/* Outline ring behind the dot */}
      <mesh renderOrder={1}>
        <sphereGeometry args={[0.08, 16, 16]} />
        <meshBasicMaterial color="black" depthTest={false} />
      </mesh>
      {/* Sphere */}
      <mesh
        ref={meshRef}
        renderOrder={2}
        onPointerOver={() => setHovered(true)}
        onPointerOut={() => setHovered(false)}
      >
        <sphereGeometry args={[0.06, 16, 16]} />
        <meshStandardMaterial color={entry.hex} emissive={entry.hex} emissiveIntensity={0.5} depthTest={false} />
      </mesh>
      {/* Label (billboarded, renders above everything) */}
      {hovered && (
        <Billboard position={[0, 0.15, 0]} renderOrder={10}>
          <Text
            fontSize={0.08}
            color="white"
            anchorX="center"
            anchorY="bottom"
            outlineWidth={0.01}
            outlineColor="black"
          >
            {entry.label}
            <meshBasicMaterial color="white" depthTest={false} depthWrite={false} />
          </Text>
        </Billboard>
      )}
    </group>
  );
}

/** Hue label positioned just outside the bottom ring. */
function HueLabel({ hue }: { hue: number }) {
  const [x, y, z] = hslToPosition(hue, 1.2, 0);
  return (
    <Billboard position={[x, y, z]}>
      <Text fontSize={0.07} color="#666" anchorX="center" anchorY="middle" renderOrder={10} material-depthTest={false}>
        {hue}°
      </Text>
    </Billboard>
  );
}

function Scene({ entries }: { entries: ColorEntry[] }) {
  return (
    <>
      <ambientLight intensity={0.6} />
      <directionalLight position={[3, 5, 3]} intensity={0.8} />

      {/* HSL color volume */}
      <HslVolume />

      {/* Wireframe cylinder */}
      {[0, 0.25, 0.5, 0.75, 1].map((l) => (
        <WireRing key={l} lightness={l} opacity={l === 0 || l === 1 ? 0.15 : 0.06} />
      ))}
      {[0, 60, 120, 180, 240, 300].map((hue) => (
        <Strut key={hue} hue={hue} />
      ))}

      {/* Center axis */}
      <Line
        points={[new THREE.Vector3(0, -CYLINDER_HEIGHT / 2, 0), new THREE.Vector3(0, CYLINDER_HEIGHT / 2, 0)]}
        color="#888" lineWidth={0.5} opacity={0.12} transparent
        dashed dashSize={0.05} gapSize={0.05}
      />

      {/* Axis labels */}
      <Billboard position={[0.15, CYLINDER_HEIGHT / 2 + 0.1, 0]}>
        <Text fontSize={0.07} color="#666" anchorX="left" renderOrder={10} material-depthTest={false}>
          L=100%
        </Text>
      </Billboard>
      <Billboard position={[0.15, -CYLINDER_HEIGHT / 2 - 0.1, 0]}>
        <Text fontSize={0.07} color="#666" anchorX="left" renderOrder={10} material-depthTest={false}>
          L=0%
        </Text>
      </Billboard>

      {/* Hue labels */}
      {[0, 60, 120, 180, 240, 300].map((hue) => (
        <HueLabel key={hue} hue={hue} />
      ))}

      {/* Data points */}
      {entries.map((entry, i) => (
        <ColorPoint key={i} entry={entry} />
      ))}

      <OrbitControls
        enablePan={false}
        enableZoom={true}
        minDistance={2}
        maxDistance={8}
        autoRotate={false}
      />
    </>
  );
}

export default function ColorDiagnostics3D({ entries }: { entries: ColorEntry[] }) {
  return (
    <Canvas
      flat
      orthographic
      camera={{ position: [2.5, 1.5, 2.5], zoom: 120 }}
      style={{ width: 520, height: 480, background: "transparent" }}
    >
      <Scene entries={entries} />
    </Canvas>
  );
}
