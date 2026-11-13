"""Draws the swcode mark as the Windows icon (an ICO file) and as a phone's icons (PNG files), without any image
library.

Run: python3 scripts/make-icons.py web/public
"""

import struct
import sys
import zlib
from pathlib import Path

BLUE = (0x00, 0x80, 0xC8)
INK = (0x0B, 0x0B, 0x0C)
# The mark's two halves, in the 24 x 28 box of web/public/icon.svg.
BLUE_HALF = [(0, 7), (12, 0), (21.94, 5.8), (0, 18.6)]
INK_HALF = [(24, 9.4), (24, 21), (12, 28), (2.06, 22.2)]
MARK_WIDTH, MARK_HEIGHT = 24, 28
# A Windows icon has no safe zone: the mark fills it, on a transparent background.
WINDOWS_MARK_SHARE = 0.94
WINDOWS_SIZES = (16, 24, 32, 48, 256)
WINDOWS_ICON = "swcode.ico"
SAMPLES = 4
OPAQUE = 255
# ICO stores 256 pixels as 0, its sizes being one byte.
ICO_SIZE_BYTE = 256
ICO_HEADER_BYTES = 6
ICO_ENTRY_BYTES = 16
# Windows reads a PNG image in an ICO file at 256 pixels only: the smaller ones, which shortcuts show, are bitmaps.
PNG_ICON_SIZE = 256
BITMAP_HEADER_BYTES = 40
BITS_PER_PIXEL = 32
# Each row of the mask, one bit a pixel, takes a whole number of 32-bit words.
MASK_WORD_BITS = 32
MASK_WORD_BYTES = 4
WHITE = (0xFF, 0xFF, 0xFF)
PNG_RGB = 2
PNG_RGBA = 6
# A phone's icon of any shape: the mark alone, with a little room around it.
PHONE_MARK_SHARE = 0.82
PHONE_SIZES = (192, 512)
# A maskable icon is cut to the launcher's shape: the mark stays in the middle, inside the circle every shape keeps.
MASKABLE_ICON = ("icon-maskable-512.png", 512, 0.5)
# iOS draws an icon without transparency, and rounds its corners itself.
APPLE_ICON = ("apple-touch-icon.png", 180, 0.6)


def inside(x, y, polygon):
    hit = False
    for (x1, y1), (x2, y2) in zip(polygon, polygon[1:] + polygon[:1]):
        if (y1 > y) != (y2 > y) and x < (x2 - x1) * (y - y1) / (y2 - y1) + x1:
            hit = not hit
    return hit


def colour_at(x, y):
    """The mark's colour at a point, None outside it."""
    if inside(x, y, BLUE_HALF):
        return BLUE
    if inside(x, y, INK_HALF):
        return INK
    return None


def samples(px, py, size, share):
    """The colours under a pixel, sampled on a grid: None where the mark does not reach."""
    scale = size * share / MARK_HEIGHT
    left = (size - MARK_WIDTH * scale) / 2
    top = (size - MARK_HEIGHT * scale) / 2
    for sx in range(SAMPLES):
        for sy in range(SAMPLES):
            yield colour_at((px + (sx + 0.5) / SAMPLES - left) / scale, (py + (sy + 0.5) / SAMPLES - top) / scale)


def mark_pixel(share):
    """Draws the mark alone at a share of the picture: its colours, and the part of the pixel it covers as the alpha."""

    def draw(px, py, size):
        colours = [colour for colour in samples(px, py, size, share) if colour is not None]
        if not colours:
            return bytes(4)
        alpha = round(OPAQUE * len(colours) / SAMPLES**2)
        return bytes(round(sum(channel) / len(colours)) for channel in zip(*colours)) + bytes([alpha])

    return draw


clear_pixel = mark_pixel(WINDOWS_MARK_SHARE)


def flat_pixel(share, background):
    """Draws the mark on a flat background, which shows wherever the mark does not reach."""

    def draw(px, py, size):
        colours = [colour or background for colour in samples(px, py, size, share)]
        return bytes(round(sum(channel) / len(colours)) for channel in zip(*colours))

    return draw


def png(size, draw, colour_type):
    rows = b"".join(b"\x00" + b"".join(draw(px, py, size) for px in range(size)) for py in range(size))

    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))

    header = struct.pack(">IIBBBBB", size, size, 8, colour_type, 0, 0, 0)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", header) + chunk(b"IDAT", zlib.compress(rows, 9)) + chunk(b"IEND", b"")


def bitmap(size):
    """An ICO bitmap: a header, the BGRA rows from the bottom up, then the mask of the transparent pixels."""
    rows = [[clear_pixel(px, py, size) for px in range(size)] for py in reversed(range(size))]
    mask_row_bytes = -(-size // MASK_WORD_BITS) * MASK_WORD_BYTES
    colours = b"".join(bytes((blue, green, red, alpha)) for row in rows for red, green, blue, alpha in row)
    mask = b""
    for row in rows:
        line = bytearray(mask_row_bytes)
        for x, colour in enumerate(row):
            if colour[3] == 0:
                line[x // 8] |= 0x80 >> (x % 8)
        mask += bytes(line)
    # The height counts the colours and the mask, as ICO bitmaps do.
    header = struct.pack(
        "<IiiHHIIiiII", BITMAP_HEADER_BYTES, size, size * 2, 1, BITS_PER_PIXEL, 0, len(colours) + len(mask), 0, 0, 0, 0
    )
    return header + colours + mask


def icon_image(size):
    return png(size, clear_pixel, colour_type=PNG_RGBA) if size == PNG_ICON_SIZE else bitmap(size)


def ico():
    """An ICO file: bitmaps for the small sizes, a PNG image for the largest."""
    images = [icon_image(size) for size in WINDOWS_SIZES]
    offset = ICO_HEADER_BYTES + ICO_ENTRY_BYTES * len(images)
    entries = b""
    for size, image in zip(WINDOWS_SIZES, images):
        side = size % ICO_SIZE_BYTE
        entries += struct.pack("<BBBBHHII", side, side, 0, 0, 1, 32, len(image), offset)
        offset += len(image)
    return struct.pack("<HHH", 0, 1, len(images)) + entries + b"".join(images)


def phone_icons():
    """The icons the web app's manifest and page name: the mark alone, then on white for the shapes that cut it."""
    for size in PHONE_SIZES:
        yield f"icon-{size}.png", png(size, mark_pixel(PHONE_MARK_SHARE), colour_type=PNG_RGBA)
    for name, size, share in (MASKABLE_ICON, APPLE_ICON):
        yield name, png(size, flat_pixel(share, WHITE), colour_type=PNG_RGB)


def main():
    folder = Path(sys.argv[1])
    (folder / WINDOWS_ICON).write_bytes(ico())
    for name, image in phone_icons():
        (folder / name).write_bytes(image)


if __name__ == "__main__":
    main()
