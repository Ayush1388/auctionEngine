import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { Lot } from "../../domain/lot";
import { fetchPhotoManifest, leadPhoto } from "../../domain/photos";

export function usePhotoManifest() {
  return useQuery({
    queryKey: ["photo-manifest"],
    queryFn: ({ signal }) => fetchPhotoManifest(signal),
    staleTime: Infinity,
  });
}

interface LotPhotoProps {
  lot: Pick<Lot, "id" | "photoSlug" | "year" | "makeModel" | "title">;
  /** The `sizes` attribute for this usage (plan.md 11). */
  sizes: string;
  /** stage: featured or lot stage; card: grid card; thumb: small list and selector images. */
  variant: "stage" | "card" | "thumb";
  /** Lead photos above the fold load eagerly with high priority. */
  priority?: boolean;
  className?: string;
}

/**
 * A lot's lead photo inside a box whose size the parent fixes (aspect ratio
 * or explicit dimensions), so nothing shifts when the file arrives. Lots with
 * no photo get the typographic placeholder: the year and the car's name, like
 * a catalogue page awaiting its plate.
 */
export function LotPhoto({ lot, sizes, variant, priority = false, className = "" }: LotPhotoProps) {
  const { data: manifest } = usePhotoManifest();
  const photo = leadPhoto(manifest, lot.photoSlug, lot.id);
  const [failed, setFailed] = useState(false);
  const [loaded, setLoaded] = useState(false);

  if (photo && !failed) {
    return (
      <div className={`relative overflow-hidden bg-bg-secondary ${className}`} style={{ backgroundColor: photo.color }}>
        <img
          src={photo.src}
          srcSet={photo.srcset}
          sizes={sizes}
          width={photo.width}
          height={photo.height}
          alt={variant === "thumb" ? "" : photo.alt}
          loading={priority ? "eager" : "lazy"}
          decoding="async"
          fetchPriority={priority ? "high" : "auto"}
          onLoad={() => setLoaded(true)}
          onError={() => setFailed(true)}
          className={`absolute inset-0 size-full object-cover transition-opacity duration-[180ms] ${loaded ? "opacity-100" : "opacity-0"}`}
        />
      </div>
    );
  }

  return (
    <div
      aria-hidden="true"
      className={`grid place-items-center overflow-hidden bg-bg-secondary text-center ${className}`}
    >
      {variant === "thumb" ? (
        <span className="heading text-14 text-muted">{lot.year ?? ""}</span>
      ) : (
        <div className="px-4">
          <p className={`heading text-ink-2 ${variant === "stage" ? "text-44" : "text-36"} leading-none`}>
            {lot.year ?? lot.makeModel}
          </p>
          {lot.year && (
            <p className={`meta mt-2 text-muted ${variant === "stage" ? "text-14 md:text-16" : "text-13"}`}>{lot.makeModel}</p>
          )}
        </div>
      )}
    </div>
  );
}
