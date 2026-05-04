declare module "superellipsejs" {
  export const Preset: {
    iOS: { r1: number; r2: number };
    KakaoTalk: { r1: number; r2: number };
  };
  export function calcSuperEllipsePath(w: number, h: number, r1: number, r2: number): string;
  export function getSuperEllipsePathAsDataUri(
    w: number,
    h: number,
    r1: number,
    r2: number,
  ): { id: string; svg: string; dataUri: string };
  export function svg2DataUri(data: string): string;
}
