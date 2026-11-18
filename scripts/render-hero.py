"""Generates the README hero: a voxel level map of the AgenteARQ workflow.

Usage: python3 scripts/render-hero.py /tmp/hero.svg
Rasterise the SVG to docs/assets/hero.png at 1280x640 (for example with Playwright's screenshot).
Everything is drawn from code: isometric voxels with per-voxel colour jitter and a 5x7 bitmap font.
"""
import math
import random
import sys

W, H = 1280, 640
rnd = random.Random(11)


def shade(hexc, f):
    r, g, b = (int(hexc[i:i + 2], 16) for i in (1, 3, 5))
    c = lambda v: max(0, min(255, int(round(v * f))))
    return f"#{c(r):02x}{c(g):02x}{c(b):02x}"


def jitter(hexc, amount=0.06):
    return shade(hexc, 1 + rnd.uniform(-amount, amount))


# ------------------------------------------------------------------ voxel engine
class Model:
    def __init__(self):
        self.v = {}

    def set(self, x, y, z, col, j=0.05):
        self.v[(int(x), int(y), int(z))] = jitter(col, j) if j else col

    def box(self, x0, x1, y0, y1, z0, z1, col, j=0.05):
        for x in range(x0, x1 + 1):
            for y in range(y0, y1 + 1):
                for z in range(z0, z1 + 1):
                    self.set(x, y, z, col, j)

    def ellipsoid(self, cx, cy, cz, rx, ry, rz, col, j=0.05, where=None):
        for x in range(math.floor(cx - rx), math.ceil(cx + rx) + 1):
            for y in range(math.floor(cy - ry), math.ceil(cy + ry) + 1):
                for z in range(math.floor(cz - rz), math.ceil(cz + rz) + 1):
                    d = ((x + .5 - cx) / rx) ** 2 + ((y + .5 - cy) / ry) ** 2 + ((z + .5 - cz) / rz) ** 2
                    if d <= 1.0 and (where is None or where(x, y, z)):
                        self.set(x, y, z, col, j)

    def cylinder(self, cx, cy, r, z0, z1, col, j=0.05):
        for x in range(math.floor(cx - r), math.ceil(cx + r) + 1):
            for y in range(math.floor(cy - r), math.ceil(cy + r) + 1):
                if (x + .5 - cx) ** 2 + (y + .5 - cy) ** 2 <= r * r:
                    for z in range(z0, z1 + 1):
                        self.set(x, y, z, col, j)

    def render(self, ox, oy, s):
        w, h = s, s / 2
        P = lambda x, y, z: (ox + (x - y) * w, oy + (x + y) * h - z * s)
        out = []
        vox = self.v
        for (x, y, z) in sorted(vox, key=lambda k: (k[0] + k[1] + k[2], k[2], k[0])):
            col = vox[(x, y, z)]
            if (x, y, z + 1) not in vox:
                out.append((P(x, y, z + 1), P(x + 1, y, z + 1), P(x + 1, y + 1, z + 1), P(x, y + 1, z + 1), shade(col, 1.16)))
            if (x, y + 1, z) not in vox:
                out.append((P(x, y + 1, z), P(x + 1, y + 1, z), P(x + 1, y + 1, z + 1), P(x, y + 1, z + 1), col))
            if (x + 1, y, z) not in vox:
                out.append((P(x + 1, y, z), P(x + 1, y + 1, z), P(x + 1, y + 1, z + 1), P(x + 1, y, z + 1), shade(col, 0.74)))
        return "".join(
            f'<polygon points="{a[0]:.1f},{a[1]:.1f} {b[0]:.1f},{b[1]:.1f} {c[0]:.1f},{c[1]:.1f} {d[0]:.1f},{d[1]:.1f}" fill="{f}"/>'
            for a, b, c, d, f in out)


# ------------------------------------------------------------------ 5x7 font
F = {
    "A": "01110 10001 10001 11111 10001 10001 10001", "B": "11110 10001 10001 11110 10001 10001 11110",
    "C": "01110 10001 10000 10000 10000 10001 01110", "D": "11110 10001 10001 10001 10001 10001 11110",
    "E": "11111 10000 10000 11110 10000 10000 11111", "F": "11111 10000 10000 11110 10000 10000 10000",
    "G": "01110 10001 10000 10111 10001 10001 01111", "H": "10001 10001 10001 11111 10001 10001 10001",
    "I": "01110 00100 00100 00100 00100 00100 01110", "J": "00111 00010 00010 00010 00010 10010 01100",
    "K": "10001 10010 10100 11000 10100 10010 10001", "L": "10000 10000 10000 10000 10000 10000 11111",
    "M": "10001 11011 10101 10101 10001 10001 10001", "N": "10001 10001 11001 10101 10011 10001 10001",
    "O": "01110 10001 10001 10001 10001 10001 01110", "P": "11110 10001 10001 11110 10000 10000 10000",
    "Q": "01110 10001 10001 10001 10101 10010 01101", "R": "11110 10001 10001 11110 10100 10010 10001",
    "S": "01111 10000 10000 01110 00001 00001 11110", "T": "11111 00100 00100 00100 00100 00100 00100",
    "U": "10001 10001 10001 10001 10001 10001 01110", "V": "10001 10001 10001 10001 10001 01010 00100",
    "W": "10001 10001 10001 10101 10101 10101 01010", "X": "10001 10001 01010 00100 01010 10001 10001",
    "Y": "10001 10001 01010 00100 00100 00100 00100", "Z": "11111 00001 00010 00100 01000 10000 11111",
    "0": "01110 10001 10011 10101 11001 10001 01110", "1": "00100 01100 00100 00100 00100 00100 01110",
    "2": "01110 10001 00001 00010 00100 01000 11111", "3": "11110 00001 00001 01110 00001 00001 11110",
    "4": "00010 00110 01010 10010 11111 00010 00010", "5": "11111 10000 11110 00001 00001 10001 01110",
    "6": "00110 01000 10000 11110 10001 10001 01110", "7": "11111 00001 00010 00100 01000 01000 01000",
    "8": "01110 10001 10001 01110 10001 10001 01110", "9": "01110 10001 10001 01111 00001 00010 01100",
    ".": "00000 00000 00000 00000 00000 01100 01100", "-": "00000 00000 00000 11111 00000 00000 00000",
    ":": "00000 01100 01100 00000 01100 01100 00000", "/": "00001 00010 00010 00100 01000 01000 10000",
    "!": "00100 00100 00100 00100 00100 00000 00100", ">": "01000 00100 00010 00001 00010 00100 01000",
    "=": "00000 00000 11111 00000 11111 00000 00000", ",": "00000 00000 00000 00000 01100 00100 01000", " ": "00000 00000 00000 00000 00000 00000 00000",
}


def text_w(s, k):
    return len(s) * 6 * k - k


def ptext(s, x, y, k, fill, shadow=None, anchor="start"):
    if anchor == "middle":
        x -= text_w(s, k) / 2
    rects = []
    for i, ch in enumerate(s):
        for r, row in enumerate(F[ch].split()):
            c = 0
            while c < 5:
                if row[c] == "1":
                    st = c
                    while c < 5 and row[c] == "1":
                        c += 1
                    rects.append((x + (i * 6 + st) * k, y + r * k, (c - st) * k, k))
                else:
                    c += 1
    o = []
    if shadow:
        o += [f'<rect x="{a + k:.0f}" y="{b + k:.0f}" width="{w:.0f}" height="{h:.0f}" fill="{shadow}"/>' for a, b, w, h in rects]
    o += [f'<rect x="{a:.0f}" y="{b:.0f}" width="{w:.0f}" height="{h:.0f}" fill="{fill}"/>' for a, b, w, h in rects]
    return "".join(o)


# ------------------------------------------------------------------ islands
N, C = 12, 5.5          # island footprint and centre
TOP = 3                 # first free z above the grass


def island(grass="#58c46c"):
    m = Model()
    for x in range(N):
        for y in range(N):
            d = (x + .5 - C - .5) ** 2 + (y + .5 - C - .5) ** 2
            if d > 33:
                continue
            m.set(x, y, 2, grass, 0.07)
            m.set(x, y, 1, "#8a5a33", 0.07)
            m.set(x, y, 0, "#74492a", 0.07)
            depth = int(max(0, 8 - d / 4.2) + rnd.uniform(0, 1.6))
            for z in range(-1, -depth - 1, -1):
                m.set(x, y, z, "#6b6f8e" if z > -4 else "#575a77", 0.08)
    # flowers and tufts on the rim
    for _ in range(7):
        a = rnd.uniform(0, 2 * math.pi)
        x, y = int(C + 4.3 * math.cos(a)), int(C + 4.3 * math.sin(a))
        if (x, y, 2) in m.v:
            m.set(x, y, 3, rnd.choice(["#3fa957", "#3fa957", "#f472b6", "#fde047", "#ffffff"]), 0.04)
    return m


def tree(m, x, y):
    m.box(x, x, y, y, TOP, TOP + 2, "#7c4a26")
    m.ellipsoid(x + .5, y + .5, TOP + 4.5, 2.1, 2.1, 2.2, "#2f9e55", 0.1)


# Ten levels, each something AgenteARQ actually does.
def lv_discover():
    m = island()
    m.cylinder(5.5, 5.5, 1.3, TOP, TOP + 1, "#6b7280")
    m.ellipsoid(5.5, 5.5, TOP + 5, 2.6, 2.6, 2.8, "#facc15", 0.08)
    m.set(6, 6, TOP + 6, "#fff7c2", 0)
    m.set(5, 6, TOP + 7, "#fff7c2", 0)
    tree(m, 2, 2)
    return m


def lv_specify():
    m = island()
    m.box(3, 8, 6, 6, TOP + 1, TOP + 9, "#f4e4c1")
    m.cylinder(2.5, 6.5, 0.9, TOP, TOP + 10, "#c08a4b")
    m.cylinder(9.5, 6.5, 0.9, TOP, TOP + 10, "#c08a4b")
    for z in (TOP + 3, TOP + 5, TOP + 7):
        m.box(4, 7, 6, 6, z, z, "#8b6b4a", 0.02)
    m.set(4, 6, TOP + 8, "#ef4444", 0)
    return m


def lv_design():
    m = island()
    m.box(3, 3, 5, 5, TOP, TOP + 5, "#7c4a26"); m.box(8, 8, 5, 5, TOP, TOP + 5, "#7c4a26")
    m.box(3, 8, 6, 6, TOP + 4, TOP + 9, "#2563eb", 0.03)
    for x, z in ((4, 8), (5, 8), (6, 8), (7, 8), (4, 6), (5, 7), (6, 6), (7, 6)):
        m.set(x, 6, TOP + z - 2, "#dbeafe", 0)
    tree(m, 2, 9)
    return m


def lv_plan():
    m = island()
    m.box(2, 9, 6, 6, TOP, TOP + 8, "#334155", 0.03)
    notes = [(3, 6, "#fde047"), (3, 4, "#fde047"), (5, 7, "#f472b6"), (5, 5, "#f472b6"), (7, 6, "#4ade80"), (8, 4, "#4ade80")]
    for x, z, col in notes:
        m.box(x, x + 1, 7, 7, TOP + z - 1, TOP + z, col, 0.03)
    return m


def hero(m, ox=0, oy=0):
    def S(x, y, z, col, j=0.04):
        m.set(x + ox, y + oy, z, col, j)
    for fx in (4, 6):                                   # boots and legs
        S(fx, 6, TOP, "#334155"); S(fx, 5, TOP, "#334155"); S(fx, 5, TOP + 1, "#94a3b8")
    m.ellipsoid(5.5 + ox, 5.5 + oy, TOP + 4.2, 2.4, 1.9, 2.4, "#e2e8f0", 0.04)   # armoured body
    for x in range(3 + ox, 9 + ox):                     # belt
        for y in range(3 + oy, 9 + oy):
            if (x, y, TOP + 2) in m.v:
                m.set(x, y, TOP + 2, "#475569", 0.02)
    S(5, 7, TOP + 4, "#22d3ee", 0); S(6, 7, TOP + 4, "#22d3ee", 0)                # chest light
    for z in (TOP + 3, TOP + 4):                        # arms
        S(2, 5, z, "#cbd5e1"); S(8, 6, z, "#cbd5e1")
    S(8, 7, TOP + 3, "#cbd5e1"); S(9, 7, TOP + 3, "#fbbf24", 0)                    # hand with a tool
    for x in range(3 + ox, 9 + ox):                     # scarf
        for y in range(3 + oy, 9 + oy):
            if ((x + .5 - 5.5 - ox) ** 2 + (y + .5 - 5.5 - oy) ** 2) <= 5.5:
                m.set(x, y, TOP + 7, "#ef4444", 0.06)
    S(3, 7, TOP + 6, "#ef4444"); S(2, 8, TOP + 5, "#dc2626"); S(2, 9, TOP + 5, "#ef4444")
    m.ellipsoid(5.5 + ox, 5.5 + oy, TOP + 10.2, 3.0, 2.8, 2.8, "#f1f5f9", 0.03)   # round helmet
    for x in range(2 + ox, 10 + ox):                    # visor across the front
        for z in (TOP + 9, TOP + 10):
            for y in range(9 + oy, 2 + oy, -1):
                if (x, y, z) in m.v:
                    m.set(x, y, z, "#0e7490", 0.02)
                    break
    S(4, 8, TOP + 10, "#67e8f9", 0); S(6, 8, TOP + 10, "#67e8f9", 0)              # eyes
    S(5, 5, TOP + 13, "#94a3b8"); S(5, 5, TOP + 14, "#fde047", 0)                 # antenna


def lv_build():
    m = island()
    hero(m)
    m.box(8, 10, 2, 3, TOP, TOP, "#1f2937", 0.02)      # laptop
    m.box(8, 10, 2, 2, TOP + 1, TOP + 2, "#111827", 0.02)
    m.set(9, 2, TOP + 2, "#4ade80", 0); m.set(8, 2, TOP + 1, "#4ade80", 0)
    return m


def lv_guardrails():
    m = island()
    m.ellipsoid(5.5, 6.5, TOP + 5, 3.6, 0.9, 4.6, "#1d4ed8", 0.04, where=lambda x, y, z: z >= TOP)
    for x in range(1, 11):
        for z in range(TOP, TOP + 11):
            ring = [(x, 6, z), (x, 7, z)]
            for v in ring:
                if v in m.v:
                    edge = any((x + dx, v[1], z + dz) not in m.v for dx, dz in ((1, 0), (-1, 0), (0, 1), (0, -1)))
                    if edge:
                        m.set(*v, "#fbbf24", 0.04)
    m.box(5, 6, 7, 7, TOP + 3, TOP + 7, "#f8fafc", 0.02)
    m.box(4, 7, 7, 7, TOP + 5, TOP + 6, "#f8fafc", 0.02)
    return m


def lv_verify():
    m = island(grass="#4fb3a0")
    for x, y in ((2, 2), (9, 2), (2, 9), (9, 9)):
        m.box(x, x, y, y, TOP, TOP + 8, "#22d3ee", 0.03)
    for i in range(2, 10):
        for x, y in ((i, 2), (i, 9), (2, i), (9, i)):
            m.set(x, y, TOP + 9, "#22d3ee", 0.03)
    for i in (4, 7):                                    # bars on the visible faces
        m.box(i, i, 9, 9, TOP, TOP + 8, "#67e8f9", 0.03)
        m.box(9, 9, i, i, TOP, TOP + 8, "#67e8f9", 0.03)
    m.box(4, 7, 4, 7, TOP, TOP + 2, "#7c3aed", 0.05)    # code under test
    m.box(5, 6, 9, 9, TOP + 3, TOP + 5, "#fbbf24", 0.02)  # padlock body
    m.set(5, 9, TOP + 6, "#fbbf24", 0); m.set(6, 9, TOP + 6, "#fbbf24", 0)
    m.set(5, 9, TOP + 4, "#78350f", 0)
    return m


def person(m, x, y, shirt, skin):
    m.box(x, x, y, y, TOP, TOP, "#1f2937", 0.03)
    m.box(x, x + 1, y, y, TOP + 1, TOP + 3, shirt, 0.05)
    m.ellipsoid(x + 1, y + .5, TOP + 5, 1.4, 1.2, 1.4, skin, 0.03)


def lv_review():
    m = island()
    person(m, 2, 7, "#ef4444", "#f5c9a2")
    person(m, 7, 3, "#3b82f6", "#c98d5f")
    m.ellipsoid(8.5, 8.5, TOP + 6, 1.8, 0.7, 1.8, "#e5e7eb", 0.02)   # magnifier
    m.box(9, 9, 9, 9, TOP + 1, TOP + 4, "#7c4a26", 0.02)
    return m


def lv_evidence():
    m = island()
    m.box(3, 8, 4, 7, TOP, TOP + 3, "#9a5b2c", 0.06)    # chest
    m.box(3, 8, 4, 7, TOP + 4, TOP + 5, "#a8662f", 0.06)
    for x in (3, 8):
        m.box(x, x, 4, 7, TOP, TOP + 5, "#fbbf24", 0.03)
    m.box(5, 6, 8, 8, TOP + 2, TOP + 4, "#fbbf24", 0.02)
    m.set(5, 8, TOP + 3, "#78350f", 0)
    for x, y, z in ((9, 3, TOP), (10, 4, TOP), (9, 5, TOP), (2, 9, TOP)):
        m.set(x, y, z, "#fde047", 0.05)                 # loot
    return m


def lv_ready():
    m = island(grass="#4cc96a")
    m.box(3, 8, 3, 8, TOP, TOP, "#475569", 0.03)       # plinth
    m.box(4, 7, 4, 7, TOP + 1, TOP + 1, "#64748b", 0.03)
    m.cylinder(5.5, 5.5, 0.9, TOP + 2, TOP + 3, "#ca8a04")                  # stem
    m.cylinder(5.5, 5.5, 2.8, TOP + 4, TOP + 8, "#facc15", 0.06)            # cup
    m.cylinder(5.5, 5.5, 1.8, TOP + 8, TOP + 8, "#a16207", 0.02)            # hollow rim
    for z in (TOP + 5, TOP + 6, TOP + 7):                                   # handles
        m.set(2, 5, z, "#facc15"); m.set(9, 6, z, "#facc15")
    m.set(1, 5, TOP + 6, "#facc15"); m.set(10, 6, TOP + 6, "#facc15")
    m.set(5, 8, TOP + 6, "#fff7c2", 0); m.set(6, 8, TOP + 5, "#fff7c2", 0)  # shine
    m.set(5, 5, TOP + 11, "#fde047", 0); m.set(5, 5, TOP + 12, "#fef9c3", 0)  # sparkle
    return m


LEVELS = [
    ("DISCOVER", "the real problem", lv_discover),
    ("SPECIFY", "testable criteria", lv_specify),
    ("DESIGN", "architecture ADR", lv_design),
    ("PLAN", "ordered tasks", lv_plan),
    ("BUILD", "your AI assistant", lv_build),
    ("GUARDRAILS", "46 agent controls", lv_guardrails),
    ("VERIFY", "sandbox, no network", lv_verify),
    ("REVIEW", "reviewer is not author", lv_review),
    ("EVIDENCE", "tied to the commit", lv_evidence),
    ("PR READY", "exact commit", lv_ready),
]

# ------------------------------------------------------------------ scene
o = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}">']
o.append('''<defs>
<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#151140"/><stop offset="0.55" stop-color="#2b2170"/><stop offset="1" stop-color="#4b3a8f"/></linearGradient>
<linearGradient id="ttl" gradientUnits="userSpaceOnUse" x1="40" y1="0" x2="460" y2="0"><stop offset="0" stop-color="#5eead4"/><stop offset="0.5" stop-color="#60a5fa"/><stop offset="1" stop-color="#c084fc"/></linearGradient>
<clipPath id="fr"><rect width="1280" height="640" rx="20"/></clipPath>
</defs><g clip-path="url(#fr)" shape-rendering="crispEdges">''')
o.append(f'<rect width="{W}" height="{H}" fill="url(#sky)"/>')
for _ in range(70):
    x, y = rnd.randrange(0, W, 3), rnd.randrange(0, 300, 3)
    o.append(f'<rect x="{x}" y="{y}" width="3" height="3" fill="#ffffff" fill-opacity="{rnd.choice([.2, .35, .55])}"/>')
for cx, cy, k in ((560, 70, 6), (300, 250, 5), (1180, 300, 5), (700, 600, 6)):   # pixel clouds
    for dx, dy, ww in ((0, 0, 10), (2, -1, 6), (-2, 1, 14)):
        o.append(f'<rect x="{cx + dx * k}" y="{cy + dy * k}" width="{ww * k}" height="{k}" fill="#ffffff" fill-opacity="0.07"/>')

# title block
o.append(ptext("AGENTEARQ", 40, 34, 7, "url(#ttl)", "#0b0928"))
o.append(ptext("YOUR AI WRITES THE CODE.", 40, 104, 3, "#ffffff", "#0b0928"))
o.append(ptext("AGENTEARQ MAKES IT PROVE IT.", 40, 134, 3, "#fde047", "#0b0928"))
o.append(ptext("WORKS WITH CLAUDE CODE - CODEX - GEMINI CLI", 40, 168, 2, "#c7d2fe"))

# quest HUD
hx, hy, hw, hh = 880, 24, 372, 148
o.append(f'<rect x="{hx}" y="{hy}" width="{hw}" height="{hh}" fill="#0b0928" fill-opacity="0.75"/>')
o.append(f'<rect x="{hx}" y="{hy}" width="{hw}" height="{hh}" fill="none" stroke="#fde047" stroke-width="3"/>')
o.append(ptext("QUEST: SHIP WITH PROOF", hx + 16, hy + 14, 2, "#fde047"))
rules = ["> SANDBOXED, NO NETWORK", "> FAILS CLOSED", "> NO SELF-APPROVAL", "> EDIT = BACK TO VERIFY", "> SAVE AND RESUME ANYTIME"]
for i, r in enumerate(rules):
    o.append(ptext(r, hx + 16, hy + 40 + i * 20, 2, "#e0e7ff"))

# level map: a snake, 1-5 left to right, 6-10 right to left
pos = [(120 + i * 260, 305) for i in range(5)] + [(1160 - i * 260, 522) for i in range(5)]
S = 6.0  # voxel size in pixels
# path stones between levels, drawn first, never under an island
route = [pos[0], pos[1], pos[2], pos[3], pos[4], (1238, pos[4][1]), (1238, pos[5][1]), pos[5], pos[6], pos[7], pos[8], pos[9]]
pts = []
for a, b in zip(route, route[1:]):
    n = max(1, int(math.hypot(b[0] - a[0], b[1] - a[1]) // 20))
    pts += [(a[0] + (b[0] - a[0]) * t / n, a[1] + (b[1] - a[1]) * t / n) for t in range(n)]
for x, y in pts:
    if all(math.hypot(x - cx, (y - cy) * 2) > 92 for cx, cy in pos):
        o.append(f'<rect x="{x - 5:.0f}" y="{y - 13:.0f}" width="10" height="6" fill="#fde68a"/>'
                 f'<rect x="{x - 5:.0f}" y="{y - 7:.0f}" width="10" height="3" fill="#b45309"/>')

for i, ((name, caption, build), (cx, cy)) in enumerate(zip(LEVELS, pos)):
    m = build()
    o.append(m.render(cx, cy - (N * S / 2) - 2 * S, S))
    label = f"{i + 1}.{name}"
    lw = text_w(label, 2)
    o.append(ptext(f"{i + 1}.", cx - lw / 2, cy + 56, 2, "#fde047", "#0b0928"))
    o.append(ptext(name, cx - lw / 2 + 12 * len(f"{i + 1}."), cy + 56, 2, "#4ade80" if i == 9 else "#ffffff", "#0b0928"))
    o.append(f'<text x="{cx}" y="{cy + 86}" font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" '
             f'font-size="12" fill="#c7d2fe" text-anchor="middle">{caption}</text>')

o.append("</g></svg>")
open(sys.argv[1], "w").write("".join(o))
