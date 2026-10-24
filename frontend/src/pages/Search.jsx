// Search with type-ahead, and the trending strip.
import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { Highlight } from "../components/Highlight.jsx";
import PageHead from "../components/PageHead.jsx";
import { useBackend } from "../hooks/backend.jsx";
import { useDocumentTitle, useNow } from "../hooks/use.js";
import { secondsLeft, toLot } from "../../app/model.js";
import { clock, usd } from "../../app/util.js";

const STATUS = [["ACTIVE", "Open now"], ["NOT_ACTIVE", "Opens soon"], ["COMPLETED", "Closed"], ["", "Any"]];
const BACKEND = { elasticsearch: "Elasticsearch", postgres: "PostgreSQL full-text search (Elasticsearch is not running)", demo: "the demo index" };

function ResultCard({ a, serverNow }) {
  const l = toLot(a), live = l.status === "ACTIVE", href = `/lot/${l.id}`;
  return (
    <article className="card">
      <div className="card-media"><Link className="card-link" to={href} aria-label={`Open ${l.title}`}><img src={l.img} alt={l.alt} loading="lazy" style={{ objectPosition: l.pos }} /></Link></div>
      <div className="card-body">
        <h3><Link to={href}>{a.highlight ? <Highlight html={a.highlight} /> : l.title}</Link></h3>
        <p className="spec">{l.spec}</p>
        <p className="where">{l.where}</p>
        <dl className="lot-stats">
          <div><dt>{l.current == null ? "Opening bid" : "Current bid"}</dt><dd className="bid">{usd(l.current ?? l.startingPrice)}</dd></div>
          <div><dt>{live ? "Ends in" : "Status"}</dt><dd className="time">{live ? clock(secondsLeft(l, serverNow)) : l.status === "COMPLETED" ? "Closed" : l.status === "NOT_ACTIVE" ? "Soon" : "Cancelled"}</dd></div>
        </dl>
        <p className="lot-meta"><span>{l.bids} {l.bids === 1 ? "bid" : "bids"}</span><span className="reserve">{l.reserve}</span></p>
        <Link className="btn btn-block" to={href}>{live ? "Open lot" : "View lot"}</Link>
      </div>
    </article>
  );
}

function Trending() {
  const be = useBackend();
  const [list, setList] = useState([]);
  useEffect(() => { be.trending(8).then(setList).catch(() => {}); }, [be]);   // the page works without it
  if (!list.length) return null;
  return (
    <section aria-labelledby="trH">
      <h2 className="h2" id="trH">Hot right now</h2>
      <p className="small muted">The lots with the most bids in the last hour.</p>
      <ul className="trend">
        {list.map(a => { const l = toLot(a); return (
          <li key={l.id}><Link className="tcard" to={`/lot/${l.id}`}><span className="thumb"><img src={l.img} alt="" style={{ objectPosition: l.pos }} loading="lazy" /></span>
            <span><b>{l.title}</b><span className="small muted tnum">{usd(l.current ?? l.startingPrice)}, {l.bids} {l.bids === 1 ? "bid" : "bids"}</span></span></Link></li>
        ); })}
      </ul>
    </section>
  );
}

export default function Search() {
  useDocumentTitle("Marque | Search");
  const be = useBackend();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [text, setText] = useState(params.get("q") || "");
  const query = params.get("q") || "";
  const status = params.get("status") ?? "ACTIVE";
  const [shown, setShown] = useState([]);
  const [cursor, setCursor] = useState(null);
  const [via, setVia] = useState("");
  const [state, setState] = useState("loading");
  const [error, setError] = useState(null);
  const now = useNow(1000);
  void now;

  const run = useCallback(async (more, from) => {
    setState(s => (more ? s : "loading"));
    try {
      let page, v = "";
      if (query) { page = await be.search(query, { status: status || undefined, limit: 12, cursor: more ? from : undefined }); v = BACKEND[page.backend] || page.backend; }
      else page = await be.listAuctions({ status: status || undefined, limit: 12, cursor: more ? from : undefined });
      setShown(s => (more ? s.concat(page.auctions) : page.auctions)); setCursor(page.next_cursor); setVia(v); setState("ready");
    } catch (e) { setError(e.message); setState("error"); }
  }, [be, query, status]);
  useEffect(() => { run(false); }, [run]);

  const setStatus = k => { const p = new URLSearchParams(params); if (k === "ACTIVE") p.delete("status"); else p.set("status", k); setParams(p, { replace: true }); };

  /* type-ahead */
  const [items, setItems] = useState([]);
  const [active, setActive] = useState(-1);
  const pool = useRef(null);
  const timer = useRef(0);
  const fromOpenLots = async t => {
    pool.current ||= be.allAuctions({ status: "ACTIVE" }, 2).then(l => l.map(a => ({ id: a.id, title: a.item.name }))).catch(() => []);
    const needle = t.toLowerCase();
    return (await pool.current).filter(x => x.title.toLowerCase().split(/\s+/).some(w => w.startsWith(needle)) || x.title.toLowerCase().includes(needle)).slice(0, 6);
  };
  const onInput = e => {
    const t = e.target.value; setText(t);
    clearTimeout(timer.current);
    timer.current = setTimeout(async () => {
      const q = t.trim();
      if (q.length < 2) { setItems([]); return; }
      let s = [];
      try { s = await be.suggest(q); } catch { /* fall back below */ }
      if (!s.length) s = await fromOpenLots(q);      // Elasticsearch down: the API has no type-ahead, so match the open lots here
      setActive(-1); setItems(s);
    }, 150);
  };
  const onKey = e => {
    if (!items.length) return;
    if (e.key === "ArrowDown") { e.preventDefault(); setActive(a => (a + 1) % items.length); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setActive(a => (a - 1 + items.length) % items.length); }
    else if (e.key === "Enter" && active >= 0) { e.preventDefault(); navigate(`/lot/${items[active].id}`); }
    else if (e.key === "Escape") { setItems([]); setActive(-1); }
  };
  const submit = e => { e.preventDefault(); setItems([]); const p = new URLSearchParams(params); if (text.trim()) p.set("q", text.trim()); else p.delete("q"); setParams(p); };

  const heading = query ? `Results for "${query}"` : STATUS.find(s => s[0] === status)?.[1] || "Lots";
  return (
    <main className="page">
      <PageHead title="Search">Find a car by make, model or a word from its story. A typo still finds it.</PageHead>
      <form className="searchbar" role="search" onSubmit={submit}>
        <div className="combo">
          <label className="visually" htmlFor="q">Search lots</label>
          <input id="q" type="search" role="combobox" aria-autocomplete="list" aria-expanded={items.length > 0} aria-controls="sugg" aria-activedescendant={active >= 0 ? "s" + active : undefined}
            autoComplete="off" spellCheck={false} placeholder="Try Jaguar, Hemi or Targa" value={text} onChange={onInput} onKeyDown={onKey} onBlur={() => setTimeout(() => setItems([]), 120)} />
          {items.length > 0 && (
            <ul className="sugg" id="sugg" role="listbox" aria-label="Suggestions">
              {items.map((s, i) => <li key={s.id} role="option" id={"s" + i} aria-selected={i === active} onMouseDown={() => navigate(`/lot/${s.id}`)}>{s.title}</li>)}
            </ul>
          )}
        </div>
        <button className="btn" type="submit">Search</button>
      </form>
      <div className="chips" role="group" aria-label="Show">
        {STATUS.map(([k, l]) => <button key={k} className="preset" aria-pressed={status === k} onClick={() => setStatus(k)}>{l}</button>)}
      </div>
      <Trending />
      <section aria-labelledby="resH">
        <div className="res-head"><h2 className="h2" id="resH">{heading}</h2>
          <p className="small muted mono" role="status" aria-live="polite">{state === "ready" ? (shown.length ? `${shown.length} shown${via ? ", answered by " + via : ""}` : via ? "Answered by " + via : "") : ""}</p></div>
        <div className="grid" aria-busy={state === "loading"}>
          {state === "loading" && Array.from({ length: 4 }, (_, i) => <div key={i} className="skel" style={{ height: 360 }} />)}
          {state === "error" && <div className="empty" style={{ gridColumn: "1/-1" }}><h3>Search is not available</h3><p>{error}</p><button className="btn" onClick={() => run(false)}>Try again</button></div>}
          {state === "ready" && !shown.length && <div className="empty" style={{ gridColumn: "1/-1" }}><h3>{query ? "Nothing matches that" : "No lots here"}</h3><p>{query ? "Check the spelling, or search for a make or a model instead." : "Try another filter."}</p></div>}
          {state === "ready" && shown.map(a => <ResultCard key={a.id} a={a} serverNow={be.feed.serverNow()} />)}
        </div>
        <div className="more">{cursor && state === "ready" && <button className="btn btn-line" onClick={() => run(true, cursor)}>Show more</button>}</div>
      </section>
    </main>
  );
}
