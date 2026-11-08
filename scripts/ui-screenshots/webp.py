"""Converts out/*.png to the site's screenshots: 1640px wide WebP files in
web/static/img/features/. Needs Pillow (pip install pillow)."""
import pathlib
from PIL import Image

here = pathlib.Path(__file__).resolve().parent
dest = here.parent.parent / "web" / "static" / "img" / "features"
dest.mkdir(parents=True, exist_ok=True)
for png in sorted((here / "out").glob("*.png")):
    im = Image.open(png).convert("RGB")
    width = 1640
    im = im.resize((width, round(im.height * width / im.width)), Image.LANCZOS)
    im.save(dest / (png.stem + ".webp"), "WEBP", quality=82, method=6)
    print(dest / (png.stem + ".webp"))
