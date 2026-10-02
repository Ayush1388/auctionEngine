import type { ReactNode } from "react";
import { Link } from "react-router";

/*
 * Side-profile line drawings, single weight, drawn for this product.
 * A body shape is recognised faster than a word.
 */
const stroke = {
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.5,
  strokeLinecap: "round",
  strokeLinejoin: "round",
} as const;

function ClassicCar() {
  return (
    <g {...stroke}>
      <path d="M3 20v-4.2c.4-1.6 2.2-2.6 5.5-2.9L20 12c3.6-3.9 8-6 14-6 6.600 0 11.600 2.300 16 6.500l7.500 1.400c2.200.4 3.500 1.500 3.500 3.300V20" />
      <path d="M3 20h5M20 20h22M54 20h7" />
      <path d="M24 12c2.800-2.600 6-4 10-4s8.400 1.400 11.500 4z" />
      <path d="M34.500 8v4" />
      <circle cx="14" cy="20" r="5" />
      <circle cx="48" cy="20" r="5" />
      <circle cx="14" cy="20" r="1.600" />
      <circle cx="48" cy="20" r="1.600" />
    </g>
  );
}

function ModernCar() {
  return (
    <g {...stroke}>
      <path d="M2 20v-2.600c.3-1.300 2-2.100 5.500-2.500L23 13.200c4.600-3.700 9.800-5.600 16-5.600 5.600 0 10.200 1.500 14 4.200l6.500 1.300c1.700.4 2.500 1.200 2.500 2.700V20" />
      <path d="M2 20h5M21 20h22M57 20h5" />
      <path d="M26.500 13c3.600-2.300 7.600-3.500 12.500-3.500 3.700 0 6.900.9 9.600 2.600z" />
      <path d="M45 16l5-1" />
      <circle cx="14" cy="19.500" r="5.500" />
      <circle cx="50" cy="19.500" r="5.500" />
      <circle cx="14" cy="19.500" r="2.200" />
      <circle cx="50" cy="19.500" r="2.200" />
    </g>
  );
}

function Drawing({ children }: { children: ReactNode }) {
  return (
    <svg viewBox="0 0 64 28" aria-hidden="true" className="h-auto w-12 text-ink md:w-14">
      {children}
    </svg>
  );
}

const TILES: Array<{ label: string; to: string; drawing: ReactNode }> = [
  {
    label: "All lots",
    to: "/auctions",
    drawing: (
      <svg viewBox="0 0 64 28" aria-hidden="true" className="h-auto w-12 text-ink md:w-14">
        <g transform="translate(0 -1) scale(.56)">
          <ClassicCar />
        </g>
        <g transform="translate(26 10.500) scale(.58)">
          <ModernCar />
        </g>
      </svg>
    ),
  },
  { label: "Classic", to: "/auctions?type=classic", drawing: <Drawing><ClassicCar /></Drawing> },
  { label: "Modern", to: "/auctions?type=modern", drawing: <Drawing><ModernCar /></Drawing> },
];

/** Category links: a line drawing on a tinted square, then the label. */
export function CategoryTiles() {
  return (
    <nav aria-label="Browse by category">
      <ul className="grid grid-cols-3 gap-2 md:flex md:gap-3">
        {TILES.map((tile) => (
          <li key={tile.label} className="md:w-56">
            <Link
              to={tile.to}
              className="group flex h-full flex-col overflow-hidden rounded-card border border-line bg-surface transition-colors duration-[120ms] hover:border-line-strong md:h-14 md:flex-row md:items-stretch"
            >
              <span className="grid place-items-center bg-bg-secondary px-3 py-2.5 md:w-20 md:py-0">{tile.drawing}</span>
              <span className="meta flex min-h-11 items-center justify-center px-2 text-14 text-ink group-hover:underline md:min-h-0 md:justify-start md:px-4 md:text-16">
                {tile.label}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}
