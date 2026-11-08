#!/usr/bin/env python3
"""Renders cmd/iec61850-explorer/appicon.png, the program icon.

The artwork is the IEC 61850 icon of the OT Fabric website
(frontend/src/assets/iec61850.svg): a transmission tower, copper on a
light copper tile. The geometry below is that SVG's, drawn in the layout
macOS expects of an app icon: a rounded tile of 824 px centred on a
transparent 1024 px canvas. Without the margin and the transparent corners
the Dock shows a larger, square icon next to everyone else's rounded ones.

Pure standard library, so that it runs anywhere:

    python3 packaging/icons/render_appicon.py
"""

import math
import os
import struct
import zlib

SIZE = 1024
TILE = 824                      # Apple's icon grid: 824 px tile, 100 px margin
MARGIN = (SIZE - TILE) / 2
SCALE = TILE / 48               # the SVG's viewBox is 48 x 48
RADIUS = 11 * SCALE             # the tile's rx="11"
TILE_COLOR = (0xF6, 0xE6, 0xDA)
STROKE_COLOR = (0xB3, 0x5A, 0x1F)
HALF_STROKE = 1.8 / 2 * SCALE

# The glyph, in viewBox units (the SVG group is translated by 12, 12).
SEGMENTS = [
    ((8.8, 21), (12, 3)), ((12, 3), (15.2, 21)),                  # legs
    ((5.5, 7.5), (18.5, 7.5)), ((7.2, 11.5), (16.8, 11.5)),       # cross-arms
    ((10.1, 13.5), (14.4, 18)), ((13.9, 13.5), (9.6, 18)),        # bracing
    ((6, 21), (18, 21)),                                          # ground
]


def to_px(p, offset=12):
    return (MARGIN + (p[0] + offset) * SCALE, MARGIN + (p[1] + offset) * SCALE)


SEGMENTS_PX = [(to_px(a), to_px(b)) for a, b in SEGMENTS]


def coverage(distance):
    """Anti-aliasing: how much of a pixel lies inside, from its signed distance to the edge."""
    return min(1.0, max(0.0, 0.5 - distance))


def tile_distance(x, y):
    half = TILE / 2 - RADIUS
    dx = abs(x - SIZE / 2) - half
    dy = abs(y - SIZE / 2) - half
    return math.hypot(max(dx, 0), max(dy, 0)) + min(max(dx, dy), 0) - RADIUS


def segment_distance(x, y, a, b):
    vx, vy = b[0] - a[0], b[1] - a[1]
    t = ((x - a[0]) * vx + (y - a[1]) * vy) / (vx * vx + vy * vy)
    t = min(1.0, max(0.0, t))
    return math.hypot(x - a[0] - t * vx, y - a[1] - t * vy)


def glyph_distance(x, y):
    d = math.inf
    for a, b in SEGMENTS_PX:
        d = min(d, segment_distance(x, y, a, b))
    return d - HALF_STROKE


def render():
    lo, hi = MARGIN + 12 * SCALE - HALF_STROKE - 2, MARGIN + 36 * SCALE + HALF_STROKE + 2
    rows = []
    for py in range(SIZE):
        y = py + 0.5
        row = bytearray([0])  # PNG filter type: none
        for px in range(SIZE):
            x = px + 0.5
            alpha = coverage(tile_distance(x, y))
            r, g, b = TILE_COLOR
            if alpha and lo <= x <= hi and lo <= y <= hi:
                ink = coverage(glyph_distance(x, y))
                if ink:
                    r, g, b = (round(t + (s - t) * ink) for t, s in zip(TILE_COLOR, STROKE_COLOR))
            row += bytes((r, g, b, round(alpha * 255)))
        rows.append(bytes(row))
    return b"".join(rows)


def png(pixels):
    def chunk(kind, data):
        body = kind + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body))

    header = struct.pack(">IIBBBBB", SIZE, SIZE, 8, 6, 0, 0, 0)  # 8-bit RGBA
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", header) + chunk(b"IDAT", zlib.compress(pixels, 9)) + chunk(b"IEND", b"")


if __name__ == "__main__":
    out = os.path.join(os.path.dirname(__file__), "..", "..", "cmd", "iec61850-explorer", "appicon.png")
    with open(out, "wb") as f:
        f.write(png(render()))
    print(os.path.normpath(out))
