// A strip at the top of a demo page: where this stage sits in the flow, what it is in one
// sentence, what it is built with, and the way to the neighbours.
import { Link, useLocation } from "react-router-dom";
import { STAGES, stageById } from "../../app/tour.js";

export default function TourBar({ stage }) {
  const { pathname, search } = useLocation();
  const s = stageById[stage];
  if (!s) return null;
  const prev = STAGES[s.n - 2], next = STAGES[s.n];
  const here = x => x.id === stage;
  void pathname; void search;
  return (
    <nav className="tourbar" aria-label="Backend tour">
      <div className="tourbar-top">
        <Link className="tourbar-home" to="/tour">Backend tour</Link>
        <ol className="tourbar-steps" aria-label="Stages">
          {STAGES.map(x => <li key={x.id}><Link to={x.page} aria-current={here(x) ? "step" : undefined} title={x.title}><span className="visually">Stage </span>{x.n}</Link></li>)}
        </ol>
        <span className="tourbar-nav">
          {prev && <Link className="linkish" to={prev.page}>Previous: {prev.short}</Link>}
          {next ? <Link className="linkish" to={next.page}>Next: {next.short}</Link> : <Link className="linkish" to="/tour">Back to the tour</Link>}
        </span>
      </div>
      <div className="tourbar-body">
        <p className="tourbar-what"><b>Stage {s.n} of {STAGES.length}. {s.title}.</b> {s.what}</p>
        <ul className="chips tourbar-tech" aria-label="Built with">{s.tech.map(t => <li key={t}>{t}</li>)}</ul>
      </div>
    </nav>
  );
}
