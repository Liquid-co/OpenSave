"""Generate the installer's artwork from the app's icon and palette.

The two bitmaps NSIS wants are fixed sizes it will not scale: 164x314 down
the left of the welcome and finish pages, and 150x57 in the header strip of
every page between them. They are committed next to this script because a
build should not need Python; run it again when the icon or the palette
changes.

    python make_art.py

Written as 24-bit uncompressed BMP, which is what NSIS reads.
"""

import os
from PIL import Image, ImageDraw, ImageFont

HERE = os.path.dirname(os.path.abspath(__file__))
ICON = os.path.join(HERE, "..", "..", "appicon.png")

# The app's own palette, from frontend/src/app.css. Kept in step by hand:
# there are four values and they have not moved in a year.
BG = (0x0C, 0x0C, 0x0D)          # --bg
TEXT = (0xE8, 0xE8, 0xEA)        # --text
DIM = (0x5E, 0x5E, 0x66)         # --text-faint
ACCENT = (0x8A, 0x63, 0xF4)      # --accent


def font(size, bold=False):
    """Segoe UI, the font the rest of the installer's chrome uses."""
    for name in (("segoeuib.ttf", "seguisb.ttf") if bold else ("segoeui.ttf",)):
        path = os.path.join(os.environ.get("WINDIR", r"C:\Windows"), "Fonts", name)
        if os.path.exists(path):
            return ImageFont.truetype(path, size)
    return ImageFont.load_default()


def glow(img, centre, radius, colour, strength=0.5):
    """A soft radial wash, the same trick the app's backdrop uses.

    Drawn as concentric circles on an overlay rather than with a blur, so the
    result is identical on any Pillow version.
    """
    layer = Image.new("RGB", img.size, BG)
    draw = ImageDraw.Draw(layer)
    steps = 48
    for i in range(steps, 0, -1):
        r = radius * i / steps
        t = (1 - i / steps) ** 2 * strength
        shade = tuple(int(BG[c] + (colour[c] - BG[c]) * t) for c in range(3))
        draw.ellipse(
            [centre[0] - r, centre[1] - r, centre[0] + r, centre[1] + r],
            fill=shade,
        )
    return Image.blend(img, layer, 0.85)


def wordmark(draw, centre_x, y, size):
    """"Open" in the text colour, "Save" in the accent - as the app writes it."""
    f = font(size, bold=True)
    a, b = "Open", "Save"
    wa = draw.textlength(a, font=f)
    wb = draw.textlength(b, font=f)
    x = centre_x - (wa + wb) / 2
    draw.text((x, y), a, font=f, fill=TEXT)
    draw.text((x + wa, y), b, font=f, fill=ACCENT)


def logo(size):
    im = Image.open(ICON).convert("RGBA")
    return im.resize((size, size), Image.LANCZOS)


def welcome():
    w, h = 164, 314
    img = Image.new("RGB", (w, h), BG)
    img = glow(img, (w / 2, 96), 130, ACCENT, 0.55)

    mark = logo(84)
    img.paste(mark, (int(w / 2 - 42), 54), mark)

    draw = ImageDraw.Draw(img)
    wordmark(draw, w / 2, 156, 19)

    tag = "peer-to-peer game save sync"
    f = font(10)
    draw.text((w / 2 - draw.textlength(tag, font=f) / 2, 184), tag, font=f, fill=DIM)

    # A hairline down the inside edge, so the panel reads as part of the
    # window rather than a picture dropped into it.
    draw.line([(w - 1, 0), (w - 1, h)], fill=(0x23, 0x23, 0x28))
    img.save(os.path.join(HERE, "welcome.bmp"))


def header():
    w, h = 150, 57
    img = Image.new("RGB", (w, h), BG)
    img = glow(img, (w - 26, h / 2), 52, ACCENT, 0.45)

    mark = logo(34)
    img.paste(mark, (w - 34 - 12, int(h / 2 - 17)), mark)

    draw = ImageDraw.Draw(img)
    f = font(12, bold=True)
    label = "OpenSave"
    draw.text((w - 34 - 20 - draw.textlength(label, font=f), h / 2 - 9), label, font=f, fill=TEXT)
    img.save(os.path.join(HERE, "header.bmp"))


if __name__ == "__main__":
    welcome()
    header()
    print("wrote welcome.bmp (164x314) and header.bmp (150x57)")
