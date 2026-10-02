import { Link } from "react-router";

const link = "meta inline-flex min-h-11 items-center text-14 text-ink underline-offset-4 hover:underline";

export function Footer() {
  return (
    <footer className="border-t border-line">
      <div className="mx-auto flex w-full max-w-[1384px] flex-col gap-2 px-4 py-6 md:flex-row md:items-center md:justify-between md:px-6 lg:px-8">
        <nav aria-label="Footer" className="flex flex-wrap gap-x-6">
          <Link to="/help/bidding" className={link}>
            How bidding works
          </Link>
          <Link to="/help/funds" className={link}>
            Fees and funds
          </Link>
        </nav>
        <p className="meta text-13 text-muted">Bids are paid from test funds. No real payment is taken.</p>
      </div>
    </footer>
  );
}
