#!/usr/bin/env python3
"""Generate IdleTrigger application and tray icons."""
import struct, sys, os, math

OUT = sys.argv[1] if len(sys.argv) > 1 else "assets"

def make_icon(filename, r, g, b, sizes=[16, 32, 48, 256]):
    """Generate a multi-resolution .ico file."""
    images = []
    for sz in sizes:
        pixels, and_mask = render_icon(sz, r, g, b)
        bmp_sz = 40 + len(pixels) + len(and_mask)
        images.append((sz, bmp_sz, pixels, and_mask))

    # ICO header
    data = struct.pack('<HHH', 0, 1, len(images))
    # Entries
    offset = 6 + 16 * len(images)
    for sz, bmp_sz, _, _ in images:
        h = sz if sz < 256 else 0
        w = sz if sz < 256 else 0
        data += struct.pack('<BBBBHHII', w, h, 0, 0, 1, 32, bmp_sz, offset)
        offset += bmp_sz
    # Image data
    for sz, _, pixels, and_mask in images:
        bmp_h = sz * 2  # ICO stores double height
        data += struct.pack('<IiiHHIIiiii', 40, sz, bmp_h, 1, 32, 0, len(pixels), 0, 0, 0, 0)
        data += pixels + and_mask

    path = os.path.join(OUT, filename)
    with open(path, 'wb') as f:
        f.write(data)
    print(f"  {path} ({len(data)} bytes, {len(sizes)} sizes)")

def render_icon(size, r, g, b):
    """Render a single resolution frame. Returns (pixels_bgra, and_mask)."""
    cx = cy = (size - 1) / 2
    r_outer = size * 0.38
    r_inner = size * 0.27
    stem_w = max(1, size * 0.06)

    def alpha(x, y):
        # Distance from center
        d = math.sqrt((x - cx)**2 + (y - cy)**2)
        # Outer ring
        if r_inner < d <= r_outer:
            # Anti-alias edge
            outer_aa = 1.0 - max(0, min(1, (d - r_outer) / 1.5 + 0.5))
            inner_aa = max(0, min(1, (d - r_inner) / 1.5 + 0.5))
            return int(255 * min(outer_aa, inner_aa))
        # Inner fill
        if d <= r_inner:
            edge = min(1, (d - (r_inner - 2)) / 1.5 + 0.5) if r_inner > 2 else 1
            return int(255 * 0.8 * edge)
        # Power stem
        stem_top = cy - size * 0.37
        stem_bot = cy - size * 0.08
        if abs(x - cx) <= stem_w and stem_top <= y <= stem_bot and d > r_outer:
            edge_x = 1.0 - max(0, min(1, (abs(x - cx) - stem_w + 1)))
            edge_y = min(
                max(0, min(1, (y - stem_top + 1))),
                max(0, min(1, (stem_bot - y + 1)))
            )
            return int(255 * min(edge_x, edge_y))
        return 0

    # Build BGRA pixels (bottom-up for BMP)
    pixels = bytearray()
    for y in range(size - 1, -1, -1):
        for x in range(size):
            a = alpha(x, y)
            if a > 0:
                # Premultiplied-ish: scale color by alpha for smooth edges
                factor = a / 255
                pixels.extend(struct.pack('BBBB',
                    int(b * factor), int(g * factor), int(r * factor), a))
            else:
                pixels.extend(b'\x00\x00\x00\x00')

    # AND mask (1-bit transparency, row-padded to 4 bytes)
    and_mask = bytearray()
    for y in range(size - 1, -1, -1):
        row_bits = []
        for x in range(size):
            row_bits.append(0 if alpha(x, y) > 128 else 1)
        for i in range(0, size, 8):
            v = 0
            for bi in range(8):
                if i + bi < size and row_bits[i + bi]:
                    v |= 1 << (7 - bi)
            and_mask.append(v)
        rb = (size + 7) // 8
        while rb % 4:
            and_mask.append(0)
            rb += 1

    return bytes(pixels), bytes(and_mask)

# ---- generate ----
print("Generating EXE icon...")
make_icon("app.ico", 0, 180, 80)  # green, multi-res for Windows Explorer

print("Generating tray icons...")
# Colors chosen to be visible on both light and dark taskbars:
make_icon("icon_default.ico", 110, 130, 155, sizes=[32])  # slate blue
make_icon("icon_monitor.ico", 235, 160, 20, sizes=[32])   # amber
make_icon("icon_active.ico",  0, 200, 90, sizes=[32])     # green
print("Done.")
