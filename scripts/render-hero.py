"""Generates docs/assets/hero.svg: isometric voxel islands and a 5x7 bitmap font.

Usage: python3 scripts/render-hero.py docs/assets/hero.svg
"""
import sys

W, H = 1280, 640
out = []

def shade(hexc, f):
    r, g, b = (int(hexc[i:i + 2], 16) for i in (1, 3, 5))
    c = lambda v: max(0, min(255, int(v * f)))
    return f"#{c(r):02x}{c(g):02x}{c(b):02x}"

# ---------------------------------------------------------------- voxels
def render_voxels(vox, ox, oy, v):
    """vox: dict (x,y,z)->color. Isometric: x goes right-down, y goes left-down."""
    w, h = v, v / 2
    P = lambda x, y, z: (ox + (x - y) * w, oy + (x + y) * h - z * v)
    polys = []
    for (x, y, z) in sorted(vox, key=lambda k: (k[0] + k[1] + k[2], k[2])):
        col = vox[(x, y, z)]
        faces = []
        if (x, y, z + 1) not in vox:
            faces.append(([P(x, y, z + 1), P(x + 1, y, z + 1), P(x + 1, y + 1, z + 1), P(x, y + 1, z + 1)], shade(col, 1.18)))
        if (x, y + 1, z) not in vox:
            faces.append(([P(x, y + 1, z), P(x + 1, y + 1, z), P(x + 1, y + 1, z + 1), P(x, y + 1, z + 1)], col))
        if (x + 1, y, z) not in vox:
            faces.append(([P(x + 1, y, z), P(x + 1, y + 1, z), P(x + 1, y + 1, z + 1), P(x + 1, y, z + 1)], shade(col, 0.72)))
        for pts, c in faces:
            polys.append(f'<polygon points="{" ".join(f"{a:.1f},{b:.1f}" for a, b in pts)}" fill="{c}"/>')
    return "\n".join(polys)

def box(vox, x0, x1, y0, y1, z0, z1, col):
    for x in range(x0, x1 + 1):
        for y in range(y0, y1 + 1):
            for z in range(z0, z1 + 1):
                vox[(x, y, z)] = col

def island(n=6, grass="#4ade80", dirt="#8b5a2b", rock="#5b5f7a"):
    vox = {}
    box(vox, 0, n - 1, 0, n - 1, 1, 1, grass)
    box(vox, 0, n - 1, 0, n - 1, 0, 0, dirt)
    box(vox, 0, n - 1, 0, n - 1, -1, -1, rock)
    box(vox, 1, n - 2, 1, n - 2, -2, -2, shade(rock, 0.9))
    box(vox, 1, n - 2, 1, n - 2, -3, -3, shade(rock, 0.82))
    box(vox, 2, n - 3, 2, n - 3, -4, -4, shade(rock, 0.74))
    vox[(n // 2, n // 2, -5)] = shade(rock, 0.66)
    return vox

# ---------------------------------------------------------------- 5x7 font
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
 ".": "00000 00000 00000 00000 00000 01100 01100", "-": "00000 00000 00000 11111 00000 00000 00000",
 "*": "00000 00000 00100 01110 00100 00000 00000", " ": "00000 00000 00000 00000 00000 00000 00000",
}

def text_width(s, scale):
    return len(s) * 6 * scale - scale

def pixel_text(s, x, y, scale, fill, shadow=None, anchor="start"):
    if anchor == "middle":
        x -= text_width(s, scale) / 2
    rects = []
    for i, ch in enumerate(s):
        rows = F[ch].split()
        for r, row in enumerate(rows):
            c = 0
            while c < 5:
                if row[c] == "1":
                    start = c
                    while c < 5 and row[c] == "1":
                        c += 1
                    rects.append((x + (i * 6 + start) * scale, y + r * scale, (c - start) * scale, scale))
                else:
                    c += 1
    parts = []
    if shadow:
        parts += [f'<rect x="{a + scale:.0f}" y="{b + scale:.0f}" width="{w:.0f}" height="{h:.0f}" fill="{shadow}"/>' for a, b, w, h in rects]
    parts += [f'<rect x="{a:.0f}" y="{b:.0f}" width="{w:.0f}" height="{h:.0f}" fill="{fill}"/>' for a, b, w, h in rects]
    return "\n".join(parts)

# ---------------------------------------------------------------- scene
out.append(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}" role="img" aria-labelledby="t d">')
out.append('<title id="t">AgenteARQ: your AI writes the code, AgenteARQ makes it prove it</title>')
out.append('<desc id="d">Pixel-art voxel islands show seven steps from idea to a reviewed pull request: discover, specify, plan, build, verify in a sandbox, independent review, and PR ready. A voxel robot stands for the AI coding assistant. Checks run sandboxed and fail closed, review cannot come from the author, and it works with Claude Code, Codex and Gemini CLI.</desc>')
out.append('''<defs>
  <linearGradient id="sky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#0d0b26"/><stop offset="0.65" stop-color="#1b1546"/><stop offset="1" stop-color="#2a1d5c"/></linearGradient>
  <linearGradient id="title" gradientUnits="userSpaceOnUse" x1="64" y1="0" x2="560" y2="0"><stop offset="0" stop-color="#5eead4"/><stop offset="0.5" stop-color="#60a5fa"/><stop offset="1" stop-color="#c084fc"/></linearGradient>
  <clipPath id="frame"><rect width="1280" height="640" rx="20"/></clipPath>
</defs>''')
out.append('<g clip-path="url(#frame)">')
out.append(f'<rect width="{W}" height="{H}" fill="url(#sky)"/>')

# pixel stars, deterministic
import random
rnd = random.Random(7)
stars = []
for _ in range(90):
    x, y = rnd.randrange(0, W, 4), rnd.randrange(0, 330, 4)
    s = rnd.choice([2, 2, 2, 4])
    stars.append(f'<rect x="{x}" y="{y}" width="{s}" height="{s}" fill="#ffffff" fill-opacity="{rnd.choice([0.25, 0.4, 0.6, 0.85])}"/>')
out.append('<g shape-rendering="crispEdges">' + "".join(stars) + "</g>")

# pixel moon: a full disc with craters
moon = []
rows = ["0011111100", "0111111110", "1111111111", "1111111111", "1111111111", "1111111111", "1111111111", "0111111110", "0011111100"]
craters = {(2, 3), (3, 3), (6, 5), (7, 5), (6, 6), (4, 7)}
for r, row in enumerate(rows):
    for c, bit in enumerate(row):
        if bit == "1":
            col = "#e8d27a" if (c, r) in craters else "#fde68a"
            moon.append(f'<rect x="{740 + c * 5}" y="{54 + r * 5}" width="5" height="5" fill="{col}"/>')
out.append('<g shape-rendering="crispEdges">' + "".join(moon) + "</g>")

# title and taglines
out.append('<g shape-rendering="crispEdges">')
out.append(pixel_text("AGENTEARQ", 64, 52, 9, "url(#title)", shadow="#05041a"))
out.append(pixel_text("YOUR AI WRITES THE CODE.", 64, 148, 4, "#ffffff", shadow="#05041a"))
out.append(pixel_text("AGENTEARQ MAKES IT PROVE IT.", 64, 188, 4, "#fde047", shadow="#05041a"))
out.append(pixel_text("FROM IDEA TO A REVIEWED PULL REQUEST - WITH PROOF.", 64, 234, 2, "#a5b4fc"))
out.append("</g>")

# mascot robot on its own island (top right)
robot = island(6)
body, light = "#94a3b8", "#cbd5e1"
box(robot, 2, 2, 2, 2, 2, 3, "#475569"); box(robot, 4, 4, 2, 2, 2, 3, "#475569")       # legs
box(robot, 1, 5, 1, 3, 4, 7, body)                                                    # torso
robot[(3, 3, 6)] = "#4ade80"                                                          # chest light
box(robot, 2, 4, 1, 3, 8, 10, light)                                                  # head
robot[(2, 3, 9)] = "#22d3ee"; robot[(4, 3, 9)] = "#22d3ee"                            # eyes
box(robot, 6, 6, 2, 2, 5, 7, body); box(robot, 0, 0, 2, 2, 5, 7, body)                # arms
robot[(3, 2, 11)] = "#475569"; robot[(3, 2, 12)] = "#fde047"                          # antenna
out.append('<g shape-rendering="crispEdges">' + render_voxels(robot, 1060, 196, 12) + "</g>")

# stations
stations = [
    ("DISCOVER", "the real problem"), ("SPECIFY", "testable criteria"), ("PLAN", "ADR and tasks"),
    ("BUILD", "by your assistant"), ("VERIFY", "sandbox, no network"), ("REVIEW", "reviewer is not author"),
    ("PR READY", "exact commit"),
]
objs = []
v = island(6); box(v, 2, 3, 2, 3, 2, 2, "#6b7280"); box(v, 2, 3, 2, 3, 3, 4, "#facc15"); v[(2, 2, 5)] = "#fef08a"; v[(3, 3, 5)] = "#fde047"; objs.append(v)   # bulb
v = island(6); box(v, 1, 4, 3, 3, 2, 6, "#f8fafc")
for z in (3, 5):
    box(v, 1, 3, 3, 3, z, z, "#94a3b8")
objs.append(v)                                                                                              # document
v = island(6); box(v, 1, 4, 1, 4, 2, 2, "#3b82f6")
for x, y in ((1, 1), (2, 2), (3, 3), (4, 4), (1, 3), (3, 1)):
    v[(x, y, 2)] = "#bfdbfe"
v[(4, 1, 3)] = "#f97316"; v[(4, 1, 4)] = "#f97316"; objs.append(v)                                           # blueprint
v = island(6)
for x, hgt in ((1, 3), (2, 3), (3, 2), (4, 1)):
    box(v, x, x, 2, 3, 2, 1 + hgt, "#ea580c" if (x % 2) else "#f97316")
v[(4, 2, 5)] = "#fb923c"; objs.append(v)                                                                     # bricks
v = island(6)
for x, y in ((1, 1), (4, 1), (1, 4), (4, 4)):
    box(v, x, x, y, y, 2, 5, "#22d3ee")
for i in range(1, 5):
    for x, y in ((i, 1), (i, 4), (1, i), (4, i)):
        v[(x, y, 6)] = "#22d3ee"
box(v, 2, 3, 2, 3, 2, 3, "#a78bfa"); objs.append(v)                                                          # sandbox cage
v = island(6)
box(v, 1, 1, 3, 3, 2, 2, "#1f2937"); box(v, 1, 1, 3, 3, 3, 4, "#ef4444"); v[(1, 3, 5)] = "#fcd9b6"
box(v, 4, 4, 1, 1, 2, 2, "#1f2937"); box(v, 4, 4, 1, 1, 3, 4, "#3b82f6"); v[(4, 1, 5)] = "#e7b98f"
objs.append(v)                                                                                              # two people
v = island(6, grass="#22c55e"); box(v, 2, 2, 3, 3, 2, 8, "#d1d5db")
box(v, 3, 5, 3, 3, 6, 8, "#4ade80"); v[(4, 3, 7)] = "#ffffff"; objs.append(v)                                # flag

cy = 470
for i, ((label, caption), vox) in enumerate(zip(stations, objs)):
    cx = 112 + i * 176
    out.append('<g shape-rendering="crispEdges">' + render_voxels(vox, cx, cy - 40, 9) + "</g>")
    out.append('<g shape-rendering="crispEdges">' + pixel_text(label, cx, cy + 52, 3, "#4ade80" if i == 6 else "#ffffff", shadow="#05041a", anchor="middle") + "</g>")
    out.append(f'<text x="{cx}" y="{cy + 92}" font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" font-size="13" fill="#a5b4fc" text-anchor="middle">{caption}</text>')
    if i < 6:
        dots = "".join(f'<rect x="{cx + 66 + k * 12}" y="{cy - 14}" width="6" height="6" fill="#fde047"/>' for k in range(4))
        out.append('<g shape-rendering="crispEdges">' + dots + "</g>")

out.append('<g shape-rendering="crispEdges">')
out.append(pixel_text("SANDBOXED CHECKS * FAILS CLOSED * NO SELF-APPROVAL * CLAUDE CODE * CODEX * GEMINI CLI", 640, 604, 2, "#c7d2fe", anchor="middle"))
out.append("</g>")
out.append("</g></svg>")
open(sys.argv[1], "w").write("\n".join(out) + "\n")
