#!/usr/bin/env python3
"""Generate the IdleTrigger application and high-contrast tray icons."""

import binascii
import math
import os
import struct
import sys
import zlib


OUT = sys.argv[1] if len(sys.argv) > 1 else "assets"
APP_SIZES = [16, 20, 24, 32, 40, 48, 64, 128, 256]
TRAY_SIZES = [16, 20, 24, 32]


def inside_rounded_rect(x, y, left, top, right, bottom, radius):
    """Return whether a normalized point is inside a rounded rectangle."""
    px = min(max(x, left + radius), right - radius)
    py = min(max(y, top + radius), bottom - radius)
    return (x - px) ** 2 + (y - py) ** 2 <= radius**2


def inside_polygon(x, y, points):
    """Even-odd point-in-polygon test."""
    inside = False
    previous = points[-1]
    for current in points:
        x1, y1 = previous
        x2, y2 = current
        if (y1 > y) != (y2 > y):
            crossing = (x2 - x1) * (y - y1) / (y2 - y1) + x1
            if x < crossing:
                inside = not inside
        previous = current
    return inside


def mix(a, b, amount):
    return tuple(round(a[i] * (1 - amount) + b[i] * amount) for i in range(3))


def render(size, sampler):
    """Render one antialiased frame as top-down RGBA pixels."""
    supersample = 4 if size <= 64 else 3
    pixels = bytearray()

    for py in range(size):
        for px in range(size):
            alpha_sum = 0
            red_sum = green_sum = blue_sum = 0
            for sy in range(supersample):
                for sx in range(supersample):
                    x = (px + (sx + 0.5) / supersample) / size
                    y = (py + (sy + 0.5) / supersample) / size
                    red, green, blue, alpha = sampler(x, y)
                    alpha_sum += alpha
                    red_sum += red * alpha
                    green_sum += green * alpha
                    blue_sum += blue * alpha

            count = supersample * supersample
            alpha = round(alpha_sum / count)
            if alpha_sum:
                red = round(red_sum / alpha_sum)
                green = round(green_sum / alpha_sum)
                blue = round(blue_sum / alpha_sum)
            else:
                red = green = blue = 0
            pixels.extend(struct.pack("BBBB", red, green, blue, alpha))

    return bytes(pixels)


def png_chunk(kind, payload):
    """Encode one PNG chunk."""
    checksum = binascii.crc32(kind + payload) & 0xFFFFFFFF
    return struct.pack(">I", len(payload)) + kind + payload + struct.pack(">I", checksum)


def encode_png(size, pixels):
    """Encode RGBA pixels as a compact PNG icon frame."""
    stride = size * 4
    scanlines = bytearray()
    for row in range(size):
        scanlines.append(0)
        start = row * stride
        scanlines.extend(pixels[start : start + stride])

    header = struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0)
    return (
        b"\x89PNG\r\n\x1a\n"
        + png_chunk(b"IHDR", header)
        + png_chunk(b"IDAT", zlib.compress(bytes(scanlines), 9))
        + png_chunk(b"IEND", b"")
    )


def make_icon(filename, sizes, sampler):
    """Write a multi-resolution Windows ICO with PNG-compressed frames."""
    frames = []
    for size in sizes:
        frame = encode_png(size, render(size, sampler))
        frames.append((size, frame))

    data = bytearray(struct.pack("<HHH", 0, 1, len(frames)))
    offset = 6 + 16 * len(frames)
    for size, frame in frames:
        dimension = size if size < 256 else 0
        data.extend(
            struct.pack(
                "<BBBBHHII",
                dimension,
                dimension,
                0,
                0,
                1,
                32,
                len(frame),
                offset,
            )
        )
        offset += len(frame)

    for _, frame in frames:
        data.extend(frame)

    path = os.path.join(OUT, filename)
    with open(path, "wb") as icon_file:
        icon_file.write(data)
    print(f"  {path} ({len(data)} bytes, sizes: {', '.join(map(str, sizes))})")


def app_sample(x, y):
    """Full application mark: a single wake-up bolt."""
    if not inside_rounded_rect(x, y, 0.055, 0.055, 0.945, 0.945, 0.205):
        return 0, 0, 0, 0

    # A restrained cool backdrop keeps the mark legible in Explorer at 16px.
    amount = min(1, max(0, (x + y) * 0.5))
    background = mix((31, 57, 79), (13, 26, 39), amount)
    color = (*background, 255)

    if inside_rounded_rect(x, y, 0.075, 0.075, 0.925, 0.925, 0.185) and not inside_rounded_rect(
        x, y, 0.095, 0.095, 0.905, 0.905, 0.165
    ):
        color = (55, 82, 100, 255)

    bolt = [
        (0.555, 0.185),
        (0.325, 0.535),
        (0.49, 0.535),
        (0.42, 0.815),
        (0.69, 0.425),
        (0.52, 0.425),
    ]
    if inside_polygon(x, y, bolt):
        color = (81, 225, 211, 255)
    return color


def tray_sampler(accent):
    """Return the shared tiny-icon mark with a state-specific accent color."""
    dark_outline = (19, 27, 34, 255)
    light_outline = (236, 244, 245, 255)
    white = (255, 255, 255, 255)

    def sample(x, y):
        dx = x - 0.5
        dy = y - 0.515
        distance = math.hypot(dx, dy)
        if distance > 0.455:
            return 0, 0, 0, 0
        if distance > 0.405:
            return dark_outline
        if distance > 0.345:
            return light_outline
        color = (*accent, 255)

        bolt = [
            (0.535, 0.255),
            (0.35, 0.545),
            (0.485, 0.545),
            (0.435, 0.765),
            (0.66, 0.445),
            (0.52, 0.445),
        ]
        if inside_polygon(x, y, bolt):
            color = white
        return color

    return sample


os.makedirs(OUT, exist_ok=True)

print("Generating application icon...")
make_icon("app.ico", APP_SIZES, app_sample)

print("Generating high-contrast tray icons...")
make_icon("icon_default.ico", TRAY_SIZES, tray_sampler((67, 139, 202)))
make_icon("icon_monitor.ico", TRAY_SIZES, tray_sampler((224, 143, 24)))
make_icon("icon_active.ico", TRAY_SIZES, tray_sampler((20, 166, 116)))
print("Done.")
