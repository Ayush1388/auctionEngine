"""Tiny layout library for the backend-course PDFs (reportlab).

Everything visual lives here: styles, code blocks, callouts, concept cards and
the hand-drawn diagrams, so the content files stay readable.
"""
import re
from reportlab.lib import colors
from reportlab.lib.enums import TA_LEFT
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle
from reportlab.lib.units import mm
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.platypus import (BaseDocTemplate, Flowable, Frame, KeepTogether, PageBreak,
                                PageTemplate, Paragraph, Spacer, Table, TableStyle,
                                XPreformatted, CondPageBreak)
from pygments import lex
from pygments.lexers import GoLexer, BashLexer, get_lexer_by_name
from pygments.token import Token

FD = "/usr/share/fonts/truetype/dejavu/"
pdfmetrics.registerFont(TTFont("DV", FD + "DejaVuSans.ttf"))
pdfmetrics.registerFont(TTFont("DVB", FD + "DejaVuSans-Bold.ttf"))
pdfmetrics.registerFont(TTFont("DVI", FD + "DejaVuSans.ttf"))
pdfmetrics.registerFont(TTFont("DVBI", FD + "DejaVuSans-Bold.ttf"))
pdfmetrics.registerFont(TTFont("DVM", FD + "DejaVuSansMono.ttf"))
pdfmetrics.registerFont(TTFont("DVMB", FD + "DejaVuSansMono-Bold.ttf"))
pdfmetrics.registerFontFamily("DV", normal="DV", bold="DVB", italic="DVI", boldItalic="DVBI")
pdfmetrics.registerFontFamily("DVM", normal="DVM", bold="DVMB", italic="DVM", boldItalic="DVMB")

# ---- palette (matches the auction brand: cream / red / black) -------------
INK = colors.HexColor("#14171A")
RED = colors.HexColor("#AD221D")
CREAM = colors.HexColor("#F8FAED")
LINE = colors.HexColor("#D5D9C3")
MUTED = colors.HexColor("#5C6150")
TEAL = colors.HexColor("#1F5C5C")
GREEN = colors.HexColor("#2F6B3A")
AMBER = colors.HexColor("#A65F00")
BLUE = colors.HexColor("#245B8F")
CODEBG = colors.HexColor("#F3F4EA")

W, H = A4
MARGIN = 17 * mm
CW = W - 2 * MARGIN  # content width

S = {}
S["body"] = ParagraphStyle("body", fontName="DV", fontSize=9.2, leading=13.6, textColor=INK, spaceAfter=5)
S["small"] = ParagraphStyle("small", parent=S["body"], fontSize=8, leading=11, spaceAfter=2)
S["cell"] = ParagraphStyle("cell", parent=S["body"], fontSize=8.1, leading=11, spaceAfter=0)
S["cellb"] = ParagraphStyle("cellb", parent=S["cell"], fontName="DVB")
S["cellh"] = ParagraphStyle("cellh", parent=S["cell"], fontName="DVB", textColor=colors.white)
S["h1"] = ParagraphStyle("h1", fontName="DVB", fontSize=21, leading=25, textColor=INK, spaceBefore=0, spaceAfter=8)
S["h2"] = ParagraphStyle("h2", fontName="DVB", fontSize=13.2, leading=17, textColor=RED, spaceBefore=12, spaceAfter=5)
S["h3"] = ParagraphStyle("h3", fontName="DVB", fontSize=10.3, leading=14, textColor=INK, spaceBefore=8, spaceAfter=3)
for _k in ("h1", "h2", "h3"):
    S[_k].keepWithNext = 1
S["kicker"] = ParagraphStyle("kicker", fontName="DVM", fontSize=8, leading=11, textColor=RED, spaceAfter=3)
S["bullet"] = ParagraphStyle("bullet", parent=S["body"], leftIndent=13, bulletIndent=3, spaceAfter=2.5)
S["code"] = ParagraphStyle("code", fontName="DVM", fontSize=6.9, leading=9.1, textColor=INK)
S["cap"] = ParagraphStyle("cap", parent=S["body"], fontName="DVI", fontSize=7.8, leading=10, textColor=MUTED, spaceAfter=8)
S["q"] = ParagraphStyle("q", parent=S["body"], fontName="DVB", fontSize=9, leading=12.5, spaceAfter=1, spaceBefore=6)
S["a"] = ParagraphStyle("a", parent=S["body"], leftIndent=10, fontSize=8.8, leading=12.6, spaceAfter=3)
S["toc"] = ParagraphStyle("toc", parent=S["body"], fontSize=9.4, leading=14, spaceAfter=0)


def fmt(s):
    """Escape text, then **bold**, `code` -> reportlab markup."""
    s = s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
    s = re.sub(r"`([^`]+)`", lambda m: '<font name="DVM" size="8.1" color="#AD221D">%s</font>' % m.group(1), s)
    s = re.sub(r"\*\*([^*]+)\*\*", r"<b>\1</b>", s)
    return s


def P(t, st="body"):
    return Paragraph(fmt(t), S[st])


def H1(t, kicker=None):
    out = []
    if kicker:
        out.append(Paragraph(fmt(kicker), S["kicker"]))
    out.append(Paragraph(fmt(t), S["h1"]))
    return out


def H2(t):
    return [CondPageBreak(60 * mm), Paragraph(fmt(t), S["h2"])]


def H3(t):
    return [CondPageBreak(35 * mm), Paragraph(fmt(t), S["h3"])]


def bullets(items):
    return [Paragraph(fmt(i), S["bullet"], bulletText="•") for i in items]


def numbered(items):
    return [Paragraph(fmt(i), S["bullet"], bulletText=f"{n}.") for n, i in enumerate(items, 1)]


# ---- code ------------------------------------------------------------------
TOK = {
    Token.Keyword: "#AD221D", Token.Keyword.Type: "#245B8F", Token.Keyword.Declaration: "#AD221D",
    Token.Literal.String: "#2F6B3A", Token.Literal.String.Char: "#2F6B3A",
    Token.Literal.Number: "#A65F00", Token.Comment: "#7A7F6E", Token.Comment.Single: "#7A7F6E",
    Token.Comment.Multiline: "#7A7F6E", Token.Name.Builtin: "#245B8F",
    Token.Name.Function: "#14171A", Token.Operator.Word: "#AD221D",
}


def _color(tt):
    while tt is not Token:
        if tt in TOK:
            return TOK[tt]
        tt = tt.parent
    return None


def _esc(x):
    return x.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def code(text, lang="go", title=None, max_cols=104, chunk=14):
    text = text.strip("\n").expandtabs(4)
    for i, ln in enumerate(text.split("\n"), 1):
        if len(ln) > max_cols:
            print(f"  [warn] code line {i} is {len(ln)} cols: {ln[:50]}...")
    lexer = {"go": GoLexer(), "bash": BashLexer()}.get(lang)
    lines = [""]
    if lexer is None:
        lines = [_esc(x) for x in text.split("\n")]
    else:
        for tt, val in lex(text, lexer):
            c = _color(tt)
            parts = val.split("\n")
            for k, part in enumerate(parts):
                if k > 0:
                    lines.append("")
                v = _esc(part)
                lines[-1] += f'<font color="{c}">{v}</font>' if (c and part.strip()) else v
    while lines and not lines[-1].strip():
        lines.pop()
    rows = []
    if title:
        rows.append([Paragraph(f'<font name="DVM" size="7" color="#5C6150">{_esc(title)}</font>', S["small"])])
    for i in range(0, len(lines), chunk):
        rows.append([XPreformatted("\n".join(lines[i:i + chunk]), S["code"])])
    t = Table(rows, colWidths=[CW])
    st = [("BACKGROUND", (0, 0), (-1, -1), CODEBG), ("LINEBEFORE", (0, 0), (0, -1), 2.2, RED),
          ("LEFTPADDING", (0, 0), (-1, -1), 8), ("RIGHTPADDING", (0, 0), (-1, -1), 6),
          ("TOPPADDING", (0, 0), (-1, -1), 0), ("BOTTOMPADDING", (0, 0), (-1, -1), 0)]
    first = 1 if title else 0
    st += [("TOPPADDING", (0, first), (-1, first), 6), ("BOTTOMPADDING", (0, -1), (-1, -1), 6)]
    if title:
        st += [("BACKGROUND", (0, 0), (-1, 0), colors.HexColor("#E9EBDA")), ("TOPPADDING", (0, 0), (-1, 0), 4),
               ("BOTTOMPADDING", (0, 0), (-1, 0), 4)]
        st.append(("NOSPLIT", (0, 0), (0, min(1, len(rows) - 1))))
    t.setStyle(TableStyle(st))
    t.spaceAfter = 8
    t.splitByRow = 1
    return t


def grab(path, start, end=None, strip_blank_edges=True):
    """Return source text from the first line matching `start` through the
    first following line matching `end` (inclusive). Keeps the PDF honest."""
    lines = open(path).read().split("\n")
    i = next(k for k, l in enumerate(lines) if re.search(start, l))
    if end is None:
        j = len(lines) - 1
    else:
        j = next(k for k in range(i, len(lines)) if re.search(end, lines[k]))
    return "\n".join(lines[i:j + 1])


# ---- callouts and cards -----------------------------------------------------
KIND = {
    "why": ("WHY", RED), "when": ("USE IT WHEN", GREEN), "whennot": ("AVOID / CAREFUL WHEN", AMBER),
    "pros": ("ADVANTAGES", GREEN), "cons": ("DISADVANTAGES", RED), "repo": ("IN YOUR auctionEngine REPO", BLUE),
    "pitfall": ("COMMON MISTAKE", AMBER), "tip": ("REMEMBER", TEAL), "interview": ("INTERVIEW ANGLE", RED),
    "gotcha": ("CORRECTION / NUANCE", BLUE), "video": ("FROM THE VIDEO", MUTED),
}


def callout(kind, text, title=None):
    label, col = KIND[kind]
    head = Paragraph(f'<font name="DVB" size="7" color="{col.hexval().replace("0x", "#")}">{title or label}</font>', S["small"])
    items = text if isinstance(text, list) else [text]
    body = [head] + [Paragraph(fmt(x), S["cell"]) for x in items]
    t = Table([[body]], colWidths=[CW])
    t.setStyle(TableStyle([("BACKGROUND", (0, 0), (-1, -1), CREAM), ("LINEBEFORE", (0, 0), (0, -1), 2.6, col),
                           ("BOX", (0, 0), (-1, -1), 0.4, LINE), ("LEFTPADDING", (0, 0), (-1, -1), 9),
                           ("RIGHTPADDING", (0, 0), (-1, -1), 8), ("TOPPADDING", (0, 0), (-1, -1), 5),
                           ("BOTTOMPADDING", (0, 0), (-1, -1), 6)]))
    t.spaceAfter = 7
    return t


def card(name, what, why, use, notuse, pros, cons, how):
    """The 'what / why / when / when not / pros / cons / how' study card."""
    def cell(label, col, txt):
        return [Paragraph(f'<font name="DVB" size="7" color="{col.hexval().replace("0x", "#")}">{label}</font>', S["small"]),
                Paragraph(fmt(txt), S["cell"])]
    rows = [
        [cell("WHAT IT IS", INK, what), cell("WHY WE NEED IT", RED, why)],
        [cell("USE IT WHEN", GREEN, use), cell("DON'T USE IT / CAREFUL WHEN", AMBER, notuse)],
        [cell("ADVANTAGES", GREEN, pros), cell("DISADVANTAGES", RED, cons)],
        [cell("HOW (in Go / in practice)", BLUE, how), ""],
    ]
    t = Table(rows, colWidths=[CW / 2, CW / 2])
    t.setStyle(TableStyle([("SPAN", (0, 3), (1, 3)), ("BOX", (0, 0), (-1, -1), 0.6, INK),
                           ("INNERGRID", (0, 0), (-1, -1), 0.3, LINE), ("BACKGROUND", (0, 0), (-1, -1), CREAM),
                           ("VALIGN", (0, 0), (-1, -1), "TOP"), ("LEFTPADDING", (0, 0), (-1, -1), 7),
                           ("RIGHTPADDING", (0, 0), (-1, -1), 7), ("TOPPADDING", (0, 0), (-1, -1), 5),
                           ("BOTTOMPADDING", (0, 0), (-1, -1), 6)]))
    title = Paragraph(f'<font name="DVB" size="10.5" color="#14171A">{fmt(name)}</font>', S["body"])
    t.spaceAfter = 8
    return [KeepTogether([title, Spacer(1, 2), t])]


def table(data, widths, header=True, zebra=True, mono_cols=(), bold_first=False):
    rows = []
    for ri, r in enumerate(data):
        row = []
        for ci, c in enumerate(r):
            if isinstance(c, Flowable) or isinstance(c, list):
                row.append(c)
                continue
            st = "cellh" if (header and ri == 0) else ("cellb" if (bold_first and ci == 0) else "cell")
            txt = fmt(str(c))
            if ci in mono_cols and not (header and ri == 0):
                txt = f'<font name="DVM" size="7.6">{txt}</font>'
            row.append(Paragraph(txt, S[st]))
        rows.append(row)
    tot = sum(widths)
    t = Table(rows, colWidths=[w / tot * CW for w in widths], repeatRows=1 if header else 0)
    style = [("VALIGN", (0, 0), (-1, -1), "TOP"), ("GRID", (0, 0), (-1, -1), 0.3, LINE),
             ("LEFTPADDING", (0, 0), (-1, -1), 5), ("RIGHTPADDING", (0, 0), (-1, -1), 5),
             ("TOPPADDING", (0, 0), (-1, -1), 3.5), ("BOTTOMPADDING", (0, 0), (-1, -1), 4)]
    if header:
        style.append(("BACKGROUND", (0, 0), (-1, 0), INK))
    if zebra:
        for i in range(1 if header else 0, len(rows)):
            if i % 2 == 0:
                style.append(("BACKGROUND", (0, i), (-1, i), CREAM))
    t.setStyle(TableStyle(style))
    t.spaceAfter = 8
    return t


def QA(q, a):
    return [Paragraph(fmt(q), S["q"])] + [Paragraph(fmt(x), S["a"]) for x in (a if isinstance(a, list) else [a])]


# ---- diagrams -----------------------------------------------------------------
class Diagram(Flowable):
    def __init__(self, height, draw_fn, caption=None):
        super().__init__()
        self.h = height
        self.fn = draw_fn
        self.caption = caption

    def wrap(self, aw, ah):
        self.width = aw
        return aw, self.h + (14 if self.caption else 0) + 6

    def draw(self):
        c = self.canv
        cap = 14 if self.caption else 0
        c.saveState()
        c.translate(0, cap + 3)
        self.fn(c, self.width, self.h)
        c.restoreState()
        if self.caption:
            c.setFont("DVI", 7.6)
            c.setFillColor(MUTED)
            c.drawString(0, 2, self.caption)


def _arrow(c, x1, y1, x2, y2, col=INK, dashed=False, head=4.2, width=1.1):
    import math
    c.saveState()
    c.setStrokeColor(col)
    c.setFillColor(col)
    c.setLineWidth(width)
    if dashed:
        c.setDash(3, 2)
    ang = math.atan2(y2 - y1, x2 - x1)
    ex, ey = x2 - head * math.cos(ang), y2 - head * math.sin(ang)
    c.line(x1, y1, ex, ey)
    c.setDash()
    p = c.beginPath()
    p.moveTo(x2, y2)
    p.lineTo(x2 - head * 1.6 * math.cos(ang - 0.38), y2 - head * 1.6 * math.sin(ang - 0.38))
    p.lineTo(x2 - head * 1.6 * math.cos(ang + 0.38), y2 - head * 1.6 * math.sin(ang + 0.38))
    p.close()
    c.drawPath(p, fill=1, stroke=0)
    c.restoreState()


def _box(c, x, y, w, h, text, fill=CREAM, stroke=INK, tc=INK, size=7.6, bold=False, lines=None, r=3):
    c.saveState()
    c.setFillColor(fill)
    c.setStrokeColor(stroke)
    c.setLineWidth(0.9)
    c.roundRect(x, y, w, h, r, fill=1, stroke=1)
    c.setFillColor(tc)
    parts = lines or [text]
    c.setFont("DVB" if bold else "DV", size)
    lh = size + 2.2
    top = y + h / 2 + (len(parts) - 1) * lh / 2 - size * 0.32
    for i, p in enumerate(parts):
        c.drawCentredString(x + w / 2, top - i * lh, p)
    c.restoreState()


def sequence(actors, steps, height=None, caption=None, lane_colors=None, label_size=7.2):
    """actors: list of names. steps: (from, to, label[, kind]) kind in
    req|res|note|gap. Arrow labels sit above the arrow."""
    n = len(steps)
    step_h = 21
    top_pad = 34
    h = height or (top_pad + n * step_h + 8)

    def fn(c, w, hh):
        gap = w / len(actors)
        xs = [gap * (i + 0.5) for i in range(len(actors))]
        for i, a in enumerate(actors):
            col = (lane_colors or [INK] * len(actors))[i]
            bw_ = max(76, pdfmetrics.stringWidth(a, "DVB", 7.6) + 16)
            _box(c, xs[i] - bw_ / 2, hh - 22, bw_, 20, a, fill=col, stroke=col, tc=colors.white, size=7.6, bold=True)
            c.setStrokeColor(LINE)
            c.setLineWidth(1)
            c.setDash(2, 2)
            c.line(xs[i], hh - 22, xs[i], 2)
            c.setDash()
        y = hh - top_pad
        for st in steps:
            a, b, label = st[0], st[1], st[2]
            kind = st[3] if len(st) > 3 else ("req" if b > a else "res")
            if kind == "note":
                tw = pdfmetrics.stringWidth(label, "DV", label_size) + 12
                _box(c, xs[a] - 4, y - 7, max(tw, 40), 15, label, fill=colors.HexColor("#FFF6D6"), stroke=AMBER, size=label_size)
            else:
                col = RED if kind == "req" else TEAL
                x1 = xs[a] + (3 if b > a else -3)
                x2 = xs[b] + (-3 if b > a else 3)
                _arrow(c, x1, y, x2, y, col=col, dashed=(kind == "res"))
                c.setFillColor(col)
                c.setFont("DV", label_size)
                c.drawCentredString((x1 + x2) / 2, y + 3.2, label)
            y -= step_h
    return Diagram(h, fn, caption)


def flow_row(items, caption=None, box_h=34, cols=None, colors_=None, sizes=7.4):
    """Horizontal chain of boxes with arrows. items: list of (title, subtitle)."""
    n = len(items)

    def fn(c, w, hh):
        gap = 13
        bw = (w - gap * (n - 1)) / n
        for i, it in enumerate(items):
            x = i * (bw + gap)
            col = (colors_ or [INK] * n)[i]
            fill = colors.white
            t, sub = (it if isinstance(it, tuple) else (it, ""))
            c.saveState()
            c.setFillColor(CREAM)
            c.setStrokeColor(col)
            c.setLineWidth(1.1)
            c.roundRect(x, 4, bw, box_h, 3, fill=1, stroke=1)
            c.setFillColor(col)
            c.setFont("DVB", sizes)
            c.drawCentredString(x + bw / 2, 4 + box_h - 12, t)
            c.setFillColor(MUTED)
            c.setFont("DV", sizes - 1.3)
            for k, ln in enumerate(sub.split("\n")):
                c.drawCentredString(x + bw / 2, 4 + box_h - 22 - k * 8, ln)
            c.restoreState()
            if i < n - 1:
                _arrow(c, x + bw + 1, 4 + box_h / 2, x + bw + gap - 1, 4 + box_h / 2, col=INK)
    return Diagram(box_h + 10, fn, caption)


def layers(rows, caption=None, hl=None, w_frac=0.62, row_h=19):
    """Stacked boxes (OSI style). rows top->bottom: (label, note)."""
    n = len(rows)
    h = n * row_h + 6

    def fn(c, w, hh):
        bw = w * w_frac
        for i, (lab, note) in enumerate(rows):
            y = hh - (i + 1) * row_h
            on = (hl is not None and i in hl)
            _box(c, 0, y + 1, bw, row_h - 3, lab, fill=RED if on else CREAM, stroke=RED if on else INK,
                 tc=colors.white if on else INK, size=7.8, bold=on)
            c.setFillColor(RED if on else MUTED)
            c.setFont("DVB" if on else "DV", 7.4)
            c.drawString(bw + 10, y + 6, note)
    return Diagram(h, fn, caption)


def message_anatomy(lines, caption=None):
    """lines: (text, tag, color) -> monospace line + coloured bracket tag on the right."""
    lh = 14
    h = len(lines) * lh + 12

    def fn(c, w, hh):
        bw = w * 0.60
        c.setFillColor(colors.HexColor("#14171A"))
        c.roundRect(0, 0, bw, hh, 4, fill=1, stroke=0)
        for i, (txt, tag, col) in enumerate(lines):
            y = hh - 14 - i * lh
            c.setFont("DVM", 7.6)
            c.setFillColor(colors.HexColor("#F8FAED"))
            c.drawString(9, y, txt)
            if tag:
                c.setStrokeColor(col)
                c.setLineWidth(1.4)
                c.line(bw + 8, y + 3, bw + 18, y + 3)
                c.setFillColor(col)
                c.setFont("DVB", 7.4)
                c.drawString(bw + 23, y, tag)
    return Diagram(h, fn, caption)


def decision(caption=None):
    """CORS decision tree: simple request vs pre-flight."""
    def fn(c, w, hh):
        def b(x, y, bw, bh, t, fill=CREAM, stroke=INK, tc=INK, lines=None):
            _box(c, x, y, bw, bh, t, fill=fill, stroke=stroke, tc=tc, size=7.3, lines=lines)
        mid = w / 2
        b(mid - 70, hh - 24, 140, 22, "Browser JS calls fetch(url)", fill=INK, stroke=INK, tc=colors.white)
        _arrow(c, mid, hh - 24, mid, hh - 40)
        b(mid - 78, hh - 64, 156, 24, "", lines=["Is the origin different?", "(scheme + host + port)"])
        # no branch
        _arrow(c, mid - 78, hh - 52, 50, hh - 52)
        c.setFont("DVB", 7); c.setFillColor(GREEN); c.drawString(mid - 100, hh - 49, "NO")
        b(2, hh - 64, 96, 24, "", fill=colors.HexColor("#E4F0E4"), stroke=GREEN, lines=["Normal request.", "No CORS involved."])
        # yes branch
        _arrow(c, mid, hh - 64, mid, hh - 84)
        c.setFont("DVB", 7); c.setFillColor(RED); c.drawString(mid + 4, hh - 78, "YES")
        b(mid - 112, hh - 112, 224, 28, "", lines=["Method is GET/HEAD/POST AND only simple headers", "AND Content-Type is form / multipart / text/plain ?"])
        _arrow(c, mid + 112, hh - 98, w - 118, hh - 98)
        c.setFont("DVB", 7); c.setFillColor(GREEN); c.drawString(mid + 118, hh - 95, "ALL TRUE")
        b(w - 116, hh - 114, 116, 32, "", fill=colors.HexColor("#E4F0E4"), stroke=GREEN,
          lines=["SIMPLE request:", "send it, then check", "Allow-Origin on reply"])
        _arrow(c, mid, hh - 112, mid, hh - 134)
        c.setFont("DVB", 7); c.setFillColor(RED); c.drawString(mid + 4, hh - 128, "ANY FALSE  (PUT, DELETE, Authorization, JSON ...)")
        b(mid - 130, hh - 168, 260, 32, "", fill=colors.HexColor("#F6E3E1"), stroke=RED,
          lines=["PRE-FLIGHT: browser first sends OPTIONS", "with Origin + Access-Control-Request-Method/Headers,", "then the real request only if the answer allows it"])
    return Diagram(176, fn, caption)


def bars(items, caption=None, unit="KB", log=False):
    """Horizontal bar chart. items: (label, value, color)."""
    n = len(items)
    h = n * 22 + 6
    mx = max(v for _, v, _ in items)

    def fn(c, w, hh):
        left = 150
        for i, (lab, v, col) in enumerate(items):
            y = hh - (i + 1) * 22 + 4
            c.setFont("DV", 7.6)
            c.setFillColor(INK)
            c.drawRightString(left - 8, y + 4, lab)
            bw = max(2, (w - left - 90) * v / mx)
            c.setFillColor(col)
            c.rect(left, y, bw, 13, fill=1, stroke=0)
            c.setFillColor(INK)
            c.setFont("DVB", 7.6)
            c.drawString(left + bw + 6, y + 3.5, f"{v:,.0f} {unit}")
    return Diagram(h, fn, caption)


# ---- document ---------------------------------------------------------------------
class Doc(BaseDocTemplate):
    def __init__(self, path, title, running):
        super().__init__(path, pagesize=A4, leftMargin=MARGIN, rightMargin=MARGIN, topMargin=20 * mm,
                         bottomMargin=17 * mm, title=title, author="Backend from First Principles (Go)")
        self.running = running
        fr = Frame(MARGIN, 17 * mm, CW, H - 37 * mm, id="f", leftPadding=0, rightPadding=0, topPadding=0, bottomPadding=0)
        self.addPageTemplates([PageTemplate(id="p", frames=[fr], onPage=self._deco)])

    def _deco(self, c, doc):
        c.saveState()
        if doc.page > 1:
            c.setFont("DVM", 7)
            c.setFillColor(MUTED)
            c.drawString(MARGIN, H - 12 * mm, self.running)
            c.setStrokeColor(LINE)
            c.line(MARGIN, H - 13.5 * mm, W - MARGIN, H - 13.5 * mm)
            c.drawRightString(W - MARGIN, 10 * mm, f"{doc.page}")
            c.setFillColor(RED)
            c.rect(MARGIN, 10 * mm - 1, 14, 2.4, fill=1, stroke=0)
        c.restoreState()
