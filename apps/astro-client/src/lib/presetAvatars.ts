// TODO: replace with CDN base URL once available (e.g. "https://cdn.astropods.ai/avatars")
import avatar01 from "@/assets/avatars/avatar_01.svg";
import avatar02 from "@/assets/avatars/avatar_02.svg";
import avatar03 from "@/assets/avatars/avatar_03.svg";
import avatar04 from "@/assets/avatars/avatar_04.svg";
import avatar05 from "@/assets/avatars/avatar_05.svg";
import avatar06 from "@/assets/avatars/avatar_06.svg";
import avatar07 from "@/assets/avatars/avatar_07.svg";
import avatar08 from "@/assets/avatars/avatar_08.svg";
import avatar09 from "@/assets/avatars/avatar_09.svg";
import avatar10 from "@/assets/avatars/avatar_10.svg";
import avatar11 from "@/assets/avatars/avatar_11.svg";
import avatar12 from "@/assets/avatars/avatar_12.svg";
import avatar13 from "@/assets/avatars/avatar_13.svg";
import avatar14 from "@/assets/avatars/avatar_14.svg";
import avatar15 from "@/assets/avatars/avatar_15.svg";
import avatar16 from "@/assets/avatars/avatar_16.svg";
import avatar17 from "@/assets/avatars/avatar_17.svg";
import avatar18 from "@/assets/avatars/avatar_18.svg";
import avatar19 from "@/assets/avatars/avatar_19.svg";
import avatar20 from "@/assets/avatars/avatar_20.svg";
import avatar21 from "@/assets/avatars/avatar_21.svg";
import avatar22 from "@/assets/avatars/avatar_22.svg";
import avatar23 from "@/assets/avatars/avatar_23.svg";
import avatar24 from "@/assets/avatars/avatar_24.svg";
import avatar25 from "@/assets/avatars/avatar_25.svg";

export interface PresetAvatar {
  id: string;
  src: string;
  label: string;
}

export const PRESET_AVATARS: PresetAvatar[] = [
  { id: "avatar_01", src: avatar01, label: "Avatar 1" },
  { id: "avatar_02", src: avatar02, label: "Avatar 2" },
  { id: "avatar_03", src: avatar03, label: "Avatar 3" },
  { id: "avatar_04", src: avatar04, label: "Avatar 4" },
  { id: "avatar_05", src: avatar05, label: "Avatar 5" },
  { id: "avatar_06", src: avatar06, label: "Avatar 6" },
  { id: "avatar_07", src: avatar07, label: "Avatar 7" },
  { id: "avatar_08", src: avatar08, label: "Avatar 8" },
  { id: "avatar_09", src: avatar09, label: "Avatar 9" },
  { id: "avatar_10", src: avatar10, label: "Avatar 10" },
  { id: "avatar_11", src: avatar11, label: "Avatar 11" },
  { id: "avatar_12", src: avatar12, label: "Avatar 12" },
  { id: "avatar_13", src: avatar13, label: "Avatar 13" },
  { id: "avatar_14", src: avatar14, label: "Avatar 14" },
  { id: "avatar_15", src: avatar15, label: "Avatar 15" },
  { id: "avatar_16", src: avatar16, label: "Avatar 16" },
  { id: "avatar_17", src: avatar17, label: "Avatar 17" },
  { id: "avatar_18", src: avatar18, label: "Avatar 18" },
  { id: "avatar_19", src: avatar19, label: "Avatar 19" },
  { id: "avatar_20", src: avatar20, label: "Avatar 20" },
  { id: "avatar_21", src: avatar21, label: "Avatar 21" },
  { id: "avatar_22", src: avatar22, label: "Avatar 22" },
  { id: "avatar_23", src: avatar23, label: "Avatar 23" },
  { id: "avatar_24", src: avatar24, label: "Avatar 24" },
  { id: "avatar_25", src: avatar25, label: "Avatar 25" },
];

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
