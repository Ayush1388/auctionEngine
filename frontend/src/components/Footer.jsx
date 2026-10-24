import { Link } from "react-router-dom";

export default function Footer() {
  return (
    <footer className="footer">
      <div className="footer-top">
        <h2 className="wordmark small">MARQUE</h2>
        <div className="footer-cols">
          <nav aria-label="Auctions"><h3 className="mono">Auctions</h3><ul>
            <li><Link to="/#classic">Classic legends</Link></li><li><Link to="/#muscle">Midnight muscle</Link></li>
            <li><Link to="/#events">Upcoming events</Link></li><li><Link to="/search">Search and trending</Link></li></ul></nav>
          <nav aria-label="Account"><h3 className="mono">Your account</h3><ul>
            <li><Link to="/bids">My bids</Link></li><li><Link to="/wallet">Wallet and statement</Link></li><li><Link to="/sell">Sell a car</Link></li></ul></nav>
          <nav aria-label="Under the hood"><h3 className="mono">Under the hood</h3><ul>
            <li><Link to="/tour">How the backend works</Link></li><li><Link to="/status">System status</Link></li><li><Link to="/system">Architecture map</Link></li>
            <li><Link to="/chaos">Chaos lab</Link></li><li><Link to="/trust">Trust page</Link></li><li><Link to="/race">Race theater</Link></li>
            <li><Link to="/search-lab">Search lab</Link></li><li><Link to="/formats">Auction formats</Link></li><li><Link to="/admin">Operator console</Link></li>
            <li><a href="https://github.com/Ayush1388/auctionEngine#readme" rel="noopener">How it is built</a></li></ul></nav>
        </div>
      </div>
      <div className="footer-bottom"><p className="mono">© 2026 Marque Auctions. Bid responsibly.</p></div>
    </footer>
  );
}
