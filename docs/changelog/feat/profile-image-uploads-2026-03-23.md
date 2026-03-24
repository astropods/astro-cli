# Profile image upload with crop UI

## Summary

Users can now upload a custom profile image from the account settings page. The server-side pipeline (resize, optimize, S3, CDN) was already in place — this adds the client-side upload flow with a reusable image selector and cropper.

## Design

- **ImageUpload** (`components/ui/image-upload.tsx`) — Generic drag-and-drop file selector with type/size validation and circular preview. Reusable for any future image upload context (org logos, agent icons, etc.).
- **ImageCropper** (`components/ui/image-cropper.tsx`) — Wraps `react-easy-crop` with project styling. Configurable aspect ratio and crop shape. Uses `ResizeObserver` + `onMediaLoaded` to compute the minimum zoom so the image always covers the crop circle edge-to-edge. Transparent regions render as white.
- **cropImage utility** (`lib/crop-image.ts`) — Offscreen canvas crop that extracts the selected region as a JPEG blob with a white fill for transparency. Operates on the image's natural pixel coordinates so output is DPI-independent.
- **AvatarUploadDialog** (`components/settings/AvatarUploadDialog.tsx`) — Two-step dialog: select an image, then crop and upload. Composes the generic components above. Responsive layout — 85dvh on mobile, auto-height on desktop.
- **API layer** — New `uploadFormData` private method on `ApiClient` for multipart uploads (omits `Content-Type` so the browser sets the multipart boundary). Public methods: `uploadAvatar`, `setAvatarPreset`, `resetAvatar`. TanStack Query mutations (`useUploadAvatar`, `useSetAvatarPreset`, `useResetAvatar`) with session refresh on success.
- **Settings integration** — Avatar on the profile section now shows a camera overlay on hover; clicking opens the upload dialog. On success, the auth session refreshes to pick up the new `avatar_version` for CDN cache-busting.

## Migration

Run `bun install` to pick up the `react-easy-crop` dependency.
