#!/usr/bin/env python3
"""
Build a compact README animation from docs screenshot assets.
"""

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


ROOT = Path(__file__).resolve().parents[1]
SHOT_DIR = ROOT / "docs" / "assets" / "screenshots"
OUT_PATH = SHOT_DIR / "getting-started.gif"

FRAME_PLAN = [
    ("connect.png", "1) Connect hub"),
    ("admin-setup.png", "2) Sysop setup"),
    ("boards.png", "3) Boards"),
    ("chat.png", "4) Live chat"),
    ("doors.png", "5) Doors"),
    ("today.png", "6) Today brief"),
]


def with_label(image: Image.Image, label: str) -> Image.Image:
    frame = image.convert("RGB").copy()
    draw = ImageDraw.Draw(frame, "RGBA")
    font = ImageFont.load_default()

    box_h = 38
    draw.rectangle((0, 0, frame.width, box_h), fill=(12, 18, 28, 220))
    draw.text((14, 11), f"WolfBBS Quick Tour  {label}", fill=(236, 241, 248), font=font)
    return frame


def main() -> None:
    frames = []
    for filename, label in FRAME_PLAN:
        path = SHOT_DIR / filename
        if not path.exists():
            raise FileNotFoundError(f"missing screenshot: {path}")
        with Image.open(path) as image:
            frames.append(with_label(image, label))

    if not frames:
        raise RuntimeError("no frames produced")

    frames[0].save(
        OUT_PATH,
        save_all=True,
        append_images=frames[1:],
        loop=0,
        duration=[1250, 1100, 1100, 1100, 1100, 1300],
        optimize=True,
    )
    print(f"wrote {OUT_PATH}")


if __name__ == "__main__":
    main()
