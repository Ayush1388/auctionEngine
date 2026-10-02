import { Search, UserRound } from "lucide-react";
import { type FormEvent } from "react";
import { Link, NavLink, useNavigate } from "react-router";

const navLink = ({ isActive }: { isActive: boolean }) =>
  `meta inline-flex min-h-11 items-center border-b-2 px-1 text-16 text-ink transition-colors duration-[120ms] ${
    isActive ? "border-ink" : "border-transparent hover:border-line-strong"
  }`;

/** One bar: wordmark, two destinations, search, account (plan.md 3.2). */
export function Navbar() {
  const navigate = useNavigate();

  const onSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const query = String(new FormData(event.currentTarget).get("q") ?? "").trim();
    if (query) navigate(`/search?q=${encodeURIComponent(query)}`);
  };

  return (
    <header className="sticky top-0 z-40 border-b border-line bg-bg">
      <div className="mx-auto flex h-14 w-full max-w-[1384px] items-center gap-4 px-4 md:h-16 md:gap-6 md:px-6 lg:px-8">
        <Link to="/" className="wordmark inline-flex min-h-11 items-center text-20 text-ink" aria-label="Aurelian, home">
          Aurelian
        </Link>

        <nav aria-label="Main" className="hidden items-center gap-5 md:flex">
          <NavLink to="/auctions" className={navLink}>
            Auctions
          </NavLink>
          <NavLink to="/sell" className={navLink}>
            Sell
          </NavLink>
        </nav>

        <form role="search" onSubmit={onSearch} className="relative ml-auto hidden w-full max-w-md md:block lg:mx-auto">
          <label htmlFor="nav-search" className="sr-only">
            Search lots
          </label>
          <Search aria-hidden="true" size={18} strokeWidth={1.5} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted" />
          <input
            id="nav-search"
            name="q"
            type="search"
            autoComplete="off"
            placeholder="Search by make, model or year"
            className="h-11 w-full rounded-control border border-line-strong bg-surface pl-10 pr-3 text-16 text-ink placeholder:text-muted"
          />
        </form>

        <div className="ml-auto flex items-center gap-1 md:ml-0 md:gap-3">
          <Link to="/search" aria-label="Search lots" className="grid size-11 place-items-center rounded-control text-ink md:hidden">
            <Search aria-hidden="true" size={22} strokeWidth={1.5} />
          </Link>
          <Link to="/login" aria-label="Sign in" className="grid size-11 place-items-center rounded-control text-ink md:hidden">
            <UserRound aria-hidden="true" size={22} strokeWidth={1.5} />
          </Link>
          <Link to="/login" className="meta hidden min-h-11 items-center whitespace-nowrap px-2 text-16 text-ink underline-offset-4 hover:underline md:inline-flex">
            Sign in
          </Link>
          <Link
            to="/register"
            className="meta hidden min-h-11 items-center rounded-control border border-line-strong px-4 text-16 text-ink transition-colors duration-[120ms] hover:bg-bg-secondary md:inline-flex"
          >
            Register
          </Link>
        </div>
      </div>
    </header>
  );
}
