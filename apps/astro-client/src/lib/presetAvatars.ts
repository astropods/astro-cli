import { getAssetUrl } from "./assets";

const AVATAR_COUNT = 25;

export interface PresetAvatar {
  id: string;
  src: string;
  label: string;
}

export const PRESET_AVATARS: PresetAvatar[] = Array.from({ length: AVATAR_COUNT }, (_, i) => {
  const n = String(i + 1).padStart(2, "0");
  return {
    id: `avatar_${n}`,
    src: getAssetUrl(`placeholders/accounts/avatar_${n}.svg`),
    label: `Avatar ${i + 1}`,
  };
});

function hashId(id: string): number {
  let hash = 0;
  for (let i = 0; i < id.length; i++) {
    hash = (hash * 31 + id.charCodeAt(i)) >>> 0;
  }
  return hash;
}

/**
 * Get a deterministic preset avatar for an entity.
 * Always pass a stable, unique identifier (e.g. user.id or account.id).
 */
export function getPresetAvatarUrl(id: string): string {
  return PRESET_AVATARS[hashId(id) % AVATAR_COUNT].src;
}
