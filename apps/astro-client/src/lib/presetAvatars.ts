const ASSETS_BASE = import.meta.env.VITE_ASSETS_URL ?? "https://assets.astropods.ai";

export interface PresetAvatar {
  id: string;
  src: string;
  label: string;
}

export const PRESET_AVATARS: PresetAvatar[] = Array.from({ length: 25 }, (_, i) => {
  const n = String(i + 1).padStart(2, "0");
  return {
    id: `avatar_${n}`,
    src: `${ASSETS_BASE}/placeholders/accounts/avatar_${n}.svg`,
    label: `Avatar ${i + 1}`,
  };
});

function hashSeed(seed: string): number {
  let hash = 0;
  for (let i = 0; i < seed.length; i++) {
    hash = (hash * 31 + seed.charCodeAt(i)) >>> 0;
  }
  return hash;
}

export function getPresetAvatar(seed: string): PresetAvatar {
  return PRESET_AVATARS[hashSeed(seed) % PRESET_AVATARS.length];
}
