// Search lab: one query, two backends, side by side. Elasticsearch is typo tolerant and ranks; the
// PostgreSQL fallback matches whole words and prefixes. In the demo engine both are modelled. On
// the real API the page asks for the fallback by switching the Elasticsearch fault on for the
// length of one query (operators only).
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import PageHead from "../components/PageHead.jsx";
import TourBar from "../components/TourBar.jsx";
import { useBackend } from "../hooks/backend.jsx";
import { useDocumentTitle } from "../hooks/use.js";
import { Highlight } from "../components/Highlight.jsx";
import { toLot } from "../../app/model.js";
import { dur } from "../../app/util.js";

const SAMPLES = [["mustng", "a letter dropped"], ["porshe", "a letter missing"], ["corvet", "cut short"], ["mercedez", "a letter swapped"], ["boss 429", "exact words"]];
const BACKEND = b => (b === "postgres" ? "postgres" : "es");

function Column({ title, sub, r, other, running, sim, q }) {
  if (!r) return <section className="ops-sec"><h2 className="h2">{title}</h2><p className="muted">{sub}</p><div className="empty"><p>{running ? "Searching..." : "No answer from this backend."}</p></div></section>;
  if (r.error) return <section className="ops-sec"><h2 className="h2">{title}</h2><p className="form-error">{r.error}</p></section>;
  const lost = other?.hits ? other.hits.filter(a => !r.hits.some(b => b.id === a.id)) : [];
  return (
    <section className="ops-sec" aria-label={title}>
      <h2 className="h2">{title}</h2><p className="muted">{sub}</p>
      <p className="tnum"><b>{r.hits.length}</b> results in <b>{dur(r.ms)}</b>{sim ? <span className="muted"> (simulated)</span> : null}</p>
      {r.hits.length ? (
        <ol className="hits">{r.hits.map(a => <li key={a.id}><span>{a.highlight && a.highlight.includes("<em>") ? <Highlight html={a.highlight} /> : toLot(a).title}</span></li>)}</ol>
      ) : <div className="empty"><h3>Nothing found</h3><p>No whole word or prefix matches "{q}".</p></div>}
      {lost.length > 0 && <p className="small"><b>Missed here:</b> {lost.map(a => toLot(a).title).join(", ")}</p>}
    </section>
  );
}

export default function SearchLab() {
  useDocumentTitle("Marque | Search lab");
  const be = useBackend();
  const sim = be.kind === "sim";
  const [q, setQ] = useState("mustng");
  const [running, setRunning] = useState(false);
  const [es, setEs] = useState(null);
  const [pg, setPg] = useState(null);
  const [note, setNote] = useState("");
  const [err, setErr] = useState(null);
  const input = useRef(null);

  const timed = async fn => {
    const t0 = performance.now();
    try { const p = await fn(); return { ms: performance.now() - t0, backend: BACKEND(p.backend), hits: p.auctions }; }
    catch (e) { return { ms: performance.now() - t0, error: e.message }; }
  };

  const run = async term => {
    const query = (term ?? q).trim();
    if (running || !query) return;
    setRunning(true); setErr(null); setEs(null); setPg(null); setNote("");
    const o = { status: "ACTIVE", limit: 12 };
    try {
      if (sim) {
        setEs(await timed(() => be.search(query, o)));
        setPg(await timed(() => be.search(query, { ...o, force: "postgres" })));
      } else {
        const first = await timed(() => be.search(query, o));
        if (first.backend === "postgres") setPg(first); else setEs(first);
        const missing = first.backend === "postgres" ? "es" : "pg";
        const c = be.session.isAdmin ? await be.chaos.get().catch(() => null) : null;
        if (c?.enabled) {
          const want = missing === "pg";                 // to get the fallback, break Elasticsearch for one query
          const was = c.faults.find(f => f.name === "elasticsearch")?.active;
          await be.chaos.set("elasticsearch", want);
          try {
            const second = await timed(() => be.search(query, o));
            if (missing === "pg" && second.backend === "postgres") setPg(second);
            if (missing === "es" && second.backend !== "postgres") setEs(second);
          } finally { await be.chaos.set("elasticsearch", !!was); }
        } else {
          setNote(missing === "pg"
            ? "Elasticsearch answered. To see the PostgreSQL fallback here too, sign in as the operator on an API started with CHAOS_ENABLED=true, or stop Elasticsearch."
            : "Elasticsearch is not running, so only the PostgreSQL fallback answered. Start it to compare.");
        }
      }
    } catch (e) { setErr(e.message); }
    setRunning(false);
  };
  useEffect(() => { run(); }, []);   // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <main className="page ops lab">
      <TourBar stage="search" />
      <PageHead title="Search lab" action={<Link className="btn btn-line" to="/chaos">Chaos lab</Link>}>
        The same query through Elasticsearch and through the PostgreSQL fallback, side by side: what each finds, how long it takes, and what is lost when search degrades.
      </PageHead>
      <form className="lab-controls" onSubmit={e => { e.preventDefault(); run(); }}>
        <label className="field" style={{ flex: "1 1 280px" }}><span className="field-label">Search the open lots</span>
          <input ref={input} value={q} onChange={e => setQ(e.target.value)} autoComplete="off" spellCheck={false} /></label>
        <button className={"btn" + (running ? " busy" : "")} type="submit" disabled={running}>Compare</button>
      </form>
      <div className="presets" style={{ marginTop: 12 }} role="group" aria-label="Sample queries">
        {SAMPLES.map(([s, why]) => <button key={s} className="preset" aria-pressed={q === s} title={why} onClick={() => { setQ(s); run(s); }}>{s}</button>)}
      </div>
      {err && <p className="form-error" role="alert">{err}</p>}
      {note && <p className="banner warn">{note}</p>}
      <div className="split-2" style={{ marginTop: 24 }}>
        <Column title="Elasticsearch" sub="Typo tolerant: one edit per word, ranked by title match." r={es} other={pg} running={running} sim={sim} q={q} />
        <Column title="PostgreSQL fallback" sub="Full-text search: whole words and prefixes. What users get while Elasticsearch is down." r={pg} other={es} running={running} sim={sim} q={q} />
      </div>
      <p className="small muted">When Elasticsearch fails five calls in a row its breaker opens, and search is answered here until a trial request ten seconds later succeeds. The fallback is fast and always correct, but it forgives no typos.</p>
    </main>
  );
}
