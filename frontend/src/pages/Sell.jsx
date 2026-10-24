// Sell a car: create a listing, preview it, and manage my listings.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import PageHead from "../components/PageHead.jsx";
import { useBackend, useRequireAuth } from "../hooks/backend.jsx";
import { useDocumentTitle, useInterval, useNow } from "../hooks/use.js";
import { toast } from "../lib/toast.js";
import { absUrl, buildCreateRequest, loadCatalog, secondsLeft, toLot } from "../../app/model.js";
import { clock, dateTime, local, usd, uuid } from "../../app/util.js";

const SERVER_FIELDS = { "item.name": "title", "item.type": "title", "item.description": "description", starting_price: "startingPrice", min_increment: "step", starts_at: "startsAt", ends_at: "duration" };
const OPENS = [["1", "In 1 minute"], ["60", "In 1 hour"], ["1440", "In 1 day"], ["custom", "Pick a date and time"]];
const DURATIONS = [["5", "5 minutes (a quick demo)"], ["60", "1 hour"], ["360", "6 hours"], ["1440", "1 day"], ["4320", "3 days"], ["10080", "7 days"], ["20160", "14 days"]];
const num = v => Number(String(v).replace(/[^0-9]/g, "")) || 0;

function Field({ name, label, errors, hint, children }) {
  return (
    <div className={"field" + (errors[name] ? " invalid" : "")} data-f={name}>
      <label>{label}</label>{children}{hint}<span className="err">{errors[name] || ""}</span>
    </div>
  );
}

function Listings() {
  const be = useBackend();
  const [list, setList] = useState(null);
  const [err, setErr] = useState(null);
  const [cancelling, setCancelling] = useState(null);
  useNow(1000);
  const load = useCallback(async () => {
    try { setList((await be.listAuctions({ owner: "me", limit: 50 })).auctions); setErr(null); } catch (x) { setErr(x.message); }
  }, [be]);
  useEffect(() => { load(); }, [load]);
  useInterval(() => { if (list?.some(a => a.status === "NOT_ACTIVE" || a.status === "ACTIVE")) load(); }, 15000);

  const tag = a => ({ ACTIVE: ["tag-active", "Open"], NOT_ACTIVE: ["tag-upcoming", "Opens soon"], COMPLETED: ["tag-closed", "Closed"], CANCELLED: ["tag-cancelled", "Cancelled"] })[a.status] || ["", a.status];
  const cancel = async id => {
    if (!confirm("Cancel this listing? It has not opened yet, so nobody has bid.")) return;
    setCancelling(id);
    try { await be.cancelAuction(id); toast("Listing cancelled"); await load(); }
    catch (x) { toast(x.status === 409 ? "It has already opened and can no longer be cancelled" : x.message); }
    setCancelling(null);
  };
  if (err) return <p className="form-error">{err}</p>;
  if (!list) return <div className="skel" style={{ height: 90 }} />;
  if (!list.length) return <div className="empty"><h3>No listings yet</h3><p>Fill in the form and your car appears here, live, with its bids.</p></div>;
  return (
    <ul className="lrows">
      {list.map(a => {
        const l = toLot(a), [cls, label] = tag(a), cancellable = a.status === "NOT_ACTIVE" && Date.parse(a.starts_at) > Date.now();
        return (
          <li className="lrow" key={a.id}>
            <Link className="thumb" to={`/lot/${a.id}`} tabIndex={-1} aria-hidden="true"><img src={l.img} alt="" style={{ objectPosition: l.pos }} /></Link>
            <div><Link to={`/lot/${a.id}`}><b>{l.title}</b></Link>
              <p className="small muted">{l.bids} {l.bids === 1 ? "bid" : "bids"}{l.current != null ? <>, now <span className="tnum">{usd(l.current)}</span></> : null}{a.status === "ACTIVE" ? <>, ends in <span className="tnum">{clock(secondsLeft(l))}</span></> : a.status === "NOT_ACTIVE" ? <>, opens {dateTime(a.starts_at)}</> : null}</p></div>
            <div className="brow-act"><span className={"tag " + cls}>{label}</span>{cancellable && <button className={"btn btn-line btn-sm" + (cancelling === a.id ? " busy" : "")} onClick={() => cancel(a.id)}>Cancel</button>}</div>
          </li>
        );
      })}
    </ul>
  );
}

export default function Sell() {
  useDocumentTitle("Marque | Sell a car");
  const be = useBackend();
  const navigate = useNavigate();
  const { redirect } = useRequireAuth();
  const [photos, setPhotos] = useState([]);
  const [photo, setPhoto] = useState(null);
  const [localData, setLocalData] = useState(null);
  const [localPhoto, setLocalPhoto] = useState(null);
  const [upErr, setUpErr] = useState("");
  const [f, setF] = useState({ title: "", year: "", make: "", model: "", where: "", category: "classic", description: "", startingPrice: "", step: "500", opens: "1", opensAt: "", duration: "1440" });
  const [errors, setErrors] = useState({});
  const [formErr, setFormErr] = useState("");
  const [busy, setBusy] = useState(false);
  const firstBad = useRef(null);

  useEffect(() => {
    loadCatalog().then(cat => {
      const list = [...new Map(cat.lots.map(l => [l.img, { img: absUrl(l.img), pos: l.pos, alt: l.alt, key: l.key }])).values()];
      setPhotos(list); setPhoto(list[0]);
    });
  }, []);
  const set = k => e => setF(s => ({ ...s, [k]: e.target.value }));

  const startsAt = () => (f.opens === "custom" ? (f.opensAt ? new Date(f.opensAt) : null) : new Date(Date.now() + (+f.opens) * 60000 + (f.opens === "1" ? 8000 : 0)));   // a few seconds of margin: the API wants the future
  const values = () => {
    const s = startsAt(), d = +f.duration;
    return {
      title: f.title, year: num(f.year) || undefined, make: f.make, model: f.model, where: f.where, category: f.category, description: f.description,
      img: photo?.img, pos: photo?.pos, localPhoto, startingPrice: num(f.startingPrice), step: num(f.step),
      startsAt: s ? s.toISOString() : "", endsAt: s ? new Date(s.getTime() + d * 60000).toISOString() : "",
    };
  };
  const check = v => {
    const e = {}, now = new Date().getFullYear();
    if (!v.title.trim()) e.title = "Give the car a title.";
    if (!v.year || v.year < 1886 || v.year > now + 1) e.year = "Enter a year.";
    if (!v.make.trim()) e.make = "Enter the make.";
    if (!v.model.trim()) e.model = "Enter the model.";
    if (!v.where.trim()) e.where = "Say where the car is.";
    if (!v.startingPrice) e.startingPrice = "Enter an opening bid.";
    if (!v.step) e.step = "Enter the smallest raise, at least $1.";
    if (!v.startsAt) e.startsAt = "Pick when it opens.";
    return e;
  };

  const upload = async e => {
    const file = e.target.files[0];
    setUpErr("");
    if (!file) return;
    if (!/^image\//.test(file.type)) { setUpErr("Choose an image file."); return; }
    try {
      const bmp = await createImageBitmap(file), k = Math.min(1, 1400 / Math.max(bmp.width, bmp.height));
      const c = Object.assign(document.createElement("canvas"), { width: Math.round(bmp.width * k), height: Math.round(bmp.height * k) });
      c.getContext("2d").drawImage(bmp, 0, 0, c.width, c.height);
      const data = c.toDataURL("image/jpeg", 0.8), id = uuid();
      if (!local.set("marque-photo:" + id, data)) { setUpErr("This browser has no room to keep that photo. Pick a house photo instead."); return; }
      setLocalData(data); setLocalPhoto(id);
    } catch { setUpErr("That file could not be read as an image."); }
  };

  const submit = async e => {
    e.preventDefault();
    const v = values(), problems = check(v);
    setErrors(problems); setFormErr("");
    if (Object.keys(problems).length) { setTimeout(() => document.querySelector(".field.invalid input, .field.invalid select, .field.invalid textarea")?.focus(), 0); return; }
    setBusy(true);
    try {
      const a = await be.createAuction(buildCreateRequest(v));
      toast("Listed. It opens " + (new Date(a.starts_at) - Date.now() < 90000 ? "in a moment" : dateTime(a.starts_at)));
      setTimeout(() => navigate(`/lot/${a.id}`), 700);
    } catch (x) {
      setBusy(false);
      const mapped = {}; for (const [k, m] of Object.entries(x.fields || {})) mapped[SERVER_FIELDS[k] || k] = m;
      setErrors(mapped);
      if (!Object.keys(mapped).length) setFormErr(x.network ? "Cannot reach the server. Nothing was listed." : x.message);
    }
  };

  const v = useMemo(values, [f, photo, localPhoto]);   // eslint-disable-line react-hooks/exhaustive-deps
  if (redirect) return redirect;
  const img = localData || photo?.img;
  const mins = +f.duration, opensLabel = OPENS.find(o => o[0] === f.opens)[1].toLowerCase();
  void firstBad;

  return (
    <main className="page">
      <PageHead title="Sell a car" action={<a className="btn btn-line" href="#listings">My listings</a>}>List it, set the price and the clock, and bidders see it live. The auction opens and closes on its own.</PageHead>
      <div className="split sell-split">
        <form className="sell-form" onSubmit={submit} noValidate>
          <fieldset className="stack"><legend className="h2">The car</legend>
            <Field name="title" label="Title" errors={errors}><input id="title" maxLength={200} placeholder="1967 Shelby GT500 KR" autoComplete="off" value={f.title} onChange={set("title")} /></Field>
            <div className="field-row three">
              <Field name="year" label="Year" errors={errors}><input id="year" inputMode="numeric" maxLength={4} placeholder="1967" value={f.year} onChange={set("year")} /></Field>
              <Field name="make" label="Make" errors={errors}><input id="make" placeholder="Shelby" value={f.make} onChange={set("make")} /></Field>
              <Field name="model" label="Model" errors={errors}><input id="model" placeholder="GT500 KR" value={f.model} onChange={set("model")} /></Field>
            </div>
            <div className="field-row">
              <Field name="where" label="Where the car is" errors={errors}><input id="where" placeholder="Nuremberg, Germany" value={f.where} onChange={set("where")} /></Field>
              <Field name="category" label="Section" errors={errors}><select id="category" value={f.category} onChange={set("category")}><option value="classic">Classic legends</option><option value="muscle">Midnight muscle</option></select></Field>
            </div>
            <Field name="description" label="Description" errors={errors} hint={<span className="hint">{f.description.length} / 4000</span>}><textarea id="description" maxLength={4000} placeholder="History, condition, what is original." value={f.description} onChange={set("description")} /></Field>
          </fieldset>

          <fieldset className="stack"><legend className="h2">Photo</legend>
            <div className="photos" role="radiogroup" aria-label="Photo">
              {photos.map(p => (
                <label key={p.img} className={"ph" + (p === photo ? " on" : "")}>
                  <input type="radio" name="photo" className="visually" checked={p === photo} onChange={() => { setPhoto(p); setLocalPhoto(null); setLocalData(null); }} />
                  <img src={p.img} alt={p.alt} style={{ objectPosition: p.pos }} loading="lazy" />
                </label>
              ))}
            </div>
            <div className="field"><label htmlFor="upload">Or use your own</label><input id="upload" type="file" accept="image/*" onChange={upload} /><span className="hint">Your own photo is kept in this browser only, because the API stores listings, not files. Everyone else sees the house photo chosen above.</span><span className="err">{upErr}</span></div>
          </fieldset>

          <fieldset className="stack"><legend className="h2">Price and clock</legend>
            <div className="field-row">
              <Field name="startingPrice" label="Opening bid" errors={errors}><div className="money"><input id="startingPrice" inputMode="numeric" placeholder="25,000" value={f.startingPrice} onChange={set("startingPrice")} /></div></Field>
              <Field name="step" label="Each bid goes up by at least" errors={errors}><div className="money"><input id="step" inputMode="numeric" placeholder="500" value={f.step} onChange={set("step")} /></div></Field>
            </div>
            <div className="field-row">
              <Field name="startsAt" label="Opens" errors={errors}>
                <select id="opens" value={f.opens} onChange={set("opens")}>{OPENS.map(([k, l]) => <option key={k} value={k}>{l}</option>)}</select>
                {f.opens === "custom" && <input id="opensAt" type="datetime-local" aria-label="Opening date and time" value={f.opensAt} onChange={set("opensAt")} />}
              </Field>
              <Field name="duration" label="Runs for" errors={errors}><select id="duration" value={f.duration} onChange={set("duration")}>{DURATIONS.map(([k, l]) => <option key={k} value={k}>{l}</option>)}</select></Field>
            </div>
            <p className="small muted">Bids in the last two minutes move the close to two minutes after the bid, up to 10 times.</p>
          </fieldset>

          {formErr && <p className="form-error" role="alert">{formErr}</p>}
          <button className={"btn" + (busy ? " busy" : "")} disabled={busy}>List this car</button>
        </form>

        <aside className="sell-side" aria-label="Preview">
          <h2 className="h2">What bidders see</h2>
          <article className="card">
            <div className="card-media"><img src={img} alt="" style={{ objectPosition: photo?.pos }} /></div>
            <div className="card-body">
              <h3>{v.title || "Your car's title"}</h3>
              <p className="spec">{[v.make, v.model, v.year].filter(Boolean).join(" ") || "Make, model and year"}</p>
              <p className="where">{v.where || "Where the car is"}</p>
              <dl className="lot-stats"><div><dt>Opening bid</dt><dd className="bid">{usd(v.startingPrice * 100)}</dd></div><div><dt>Runs for</dt><dd className="time">{mins >= 1440 ? mins / 1440 + " d" : mins >= 60 ? mins / 60 + " h" : mins + " min"}</dd></div></dl>
              <p className="lot-meta"><span>Opens {f.opens === "custom" ? (v.startsAt ? dateTime(v.startsAt) : "on a date you choose") : opensLabel}</span><span className="reserve">{v.step ? "+" + usd(v.step * 100) + " a bid" : ""}</span></p>
            </div>
          </article>
          <section id="listings" aria-labelledby="listH">
            <h2 className="h2" id="listH">My listings</h2>
            <Listings />
          </section>
        </aside>
      </div>
    </main>
  );
}
