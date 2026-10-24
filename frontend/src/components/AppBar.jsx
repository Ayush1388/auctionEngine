import { Link, useLocation } from "react-router-dom";
import Logo from "./Logo.jsx";
import ShellActions from "./ShellActions.jsx";
import ThemeToggle from "./ThemeToggle.jsx";

export const NAV = [
  ["Classics", "/#classic"], ["Muscle", "/#muscle"], ["Events", "/#events"],
  ["Search", "/search"], ["Sell a car", "/sell"], ["Backend", "/tour"],
];

/** The bar on every page except the home page and the event pages, which draw their own over a photo. */
export default function AppBar() {
  const { pathname } = useLocation();
  const current = href => !href.includes("#") && (pathname === href || (href === "/tour" && ["/trust", "/chaos", "/system", "/race", "/search-lab", "/formats", "/status"].includes(pathname)));
  return (
    <header className="lt-bar">
      <Link className="brand" to="/" aria-label="Marque home"><Logo /></Link>
      <nav className="lt-nav" aria-label="Primary">
        {NAV.map(([t, h]) => <Link key={h} to={h} aria-current={current(h) ? "page" : undefined}>{t}</Link>)}
      </nav>
      <div className="lt-actions">
        <span className="shell-actions"><ShellActions /></span>
        <ThemeToggle />
      </div>
    </header>
  );
}
