"""Draws the Windows icon (the swcode mark alone) as an ICO file, without any image library.

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


def clear_pixel(px, py, size):
    """The mark alone: its colours, and the share of the pixel it covers as the alpha."""
    colours = [colour for colour in samples(px, py, size, WINDOWS_MARK_SHARE) if colour is not None]
    if not colours:
        return bytes(4)
    alpha = round(OPAQUE * len(colours) / SAMPLES**2)
    return bytes(round(sum(channel) / len(colours)) for channel in zip(*colours)) + bytes([alpha])


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
    return png(size, clear_pixel, colour_type=6) if size == PNG_ICON_SIZE else bitmap(size)


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


def main():
    folder = Path(sys.argv[1])
    (folder / WINDOWS_ICON).write_bytes(ico())


if __name__ == "__main__":
    main()
