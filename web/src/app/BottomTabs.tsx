import { Compass, Gavel, LayoutGrid, UserRound } from "lucide-react";
import { NavLink } from "react-router";

const TABS = [
  { to: "/", label: "Discover", icon: Compass, end: true },
  { to: "/auctions", label: "Browse", icon: LayoutGrid, end: false },
  { to: "/account/bids", label: "Bids", icon: Gavel, end: false },
  { to: "/account", label: "Account", icon: UserRound, end: true },
] as const;

/** Mobile navigation: four labelled destinations. */
export function BottomTabs() {
  return (
    <nav
      aria-label="Main"
      data-era="modern"
      className="fixed inset-x-0 bottom-0 z-40 border-t border-line pb-[env(safe-area-inset-bottom)] md:hidden"
    >
      <ul className="grid grid-cols-4">
        {TABS.map(({ to, label, icon: Icon, end }) => (
          <li key={to}>
            <NavLink
              to={to}
              end={end}
              className={({ isActive }) =>
                `meta flex min-h-14 flex-col items-center justify-center gap-0.5 text-12 ${isActive ? "text-ink" : "text-muted"}`
              }
            >
              {({ isActive }) => (
                <>
                  <Icon aria-hidden="true" size={22} strokeWidth={isActive ? 2 : 1.5} />
                  <span className={isActive ? "underline underline-offset-4" : ""}>{label}</span>
                </>
              )}
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  );
}
