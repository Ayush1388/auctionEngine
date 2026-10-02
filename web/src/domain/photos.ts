/**
 * Interim photo manifest (plan.md 3.3). The backend has no image storage
 * yet, so photos are static files listed in /lots/manifest.json and joined
 * to a lot by the "Photos: <slug>" line in its description header, or by
 * auction id. Lots without an entry get the typographic placeholder.
 */
export interface LotPhotoSource {
  /** URL of the widest file, used as the fallback src. */
  src: string;
  /** Optional srcset, e.g. "/lots/x/lead-400.avif 400w, /lots/x/lead-800.avif 800w". */
  srcset?: string;
  width: number;
  height: number;
  alt: string;
  /** Dominant colour shown while the file loads. */
  color?: string;
}

export interface ManifestEntry {
  photos: LotPhotoSource[];
}

export type PhotoManifest = Record<string, ManifestEntry>;

export async function fetchPhotoManifest(signal?: AbortSignal): Promise<PhotoManifest> {
  const response = await fetch("/lots/manifest.json", { signal });
  if (!response.ok) return {};
  return (await response.json()) as PhotoManifest;
}

export function leadPhoto(manifest: PhotoManifest | undefined, slug: string | null, id: string): LotPhotoSource | null {
  if (!manifest) return null;
  const entry = (slug ? manifest[slug] : undefined) ?? manifest[id];
  return entry?.photos[0] ?? null;
}
