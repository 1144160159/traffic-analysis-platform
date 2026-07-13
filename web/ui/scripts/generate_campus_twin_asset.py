#!/usr/bin/env python3
from __future__ import annotations

import math
from pathlib import Path

from PIL import Image, ImageDraw, ImageEnhance, ImageFilter


ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "public" / "ui-assets" / "backgrounds" / "campus-real-buildings-twin.png"
DOC_OUT = (
    ROOT.parents[1]
    / "doc"
    / "04_assets"
    / "ui_suite_gpt_v1"
    / "backgrounds"
    / "4k"
    / "campus-real-buildings-twin.png"
)
RISK_OUT = ROOT / "public" / "ui-assets" / "backgrounds" / "screen-risk-world.png"
EGRESS_OUT = ROOT / "public" / "ui-assets" / "backgrounds" / "screen-egress-world.png"
PROBE_OUT = ROOT / "public" / "ui-assets" / "backgrounds" / "screen-probe-campus-map.png"
DOC_RISK_OUT = DOC_OUT.with_name("screen-risk-world.png")
DOC_EGRESS_OUT = DOC_OUT.with_name("screen-egress-world.png")
DOC_PROBE_OUT = DOC_OUT.with_name("screen-probe-campus-map.png")
SOURCE_THREAT_MAP = ROOT.parents[1] / "doc" / "04_assets" / "ui_suite_gpt_v1" / "backgrounds" / "4k" / "bg-threat-situation-map-4k.png"

W, H = 2048, 1152
SCALE = 2


def p(x: float, y: float) -> tuple[int, int]:
    return round(x * SCALE), round(y * SCALE)


def poly(points: list[tuple[float, float]]) -> list[tuple[int, int]]:
    return [p(x, y) for x, y in points]


def glow_line(draw: ImageDraw.ImageDraw, pts: list[tuple[float, float]], color: tuple[int, int, int], width: int = 3) -> None:
    ipts = [p(x, y) for x, y in pts]
    for w, alpha in [(14, 35), (8, 65), (4, 135)]:
        layer = Image.new("RGBA", (W * SCALE, H * SCALE), (0, 0, 0, 0))
        d = ImageDraw.Draw(layer)
        d.line(ipts, fill=(*color, alpha), width=w * SCALE, joint="curve")
        layer = layer.filter(ImageFilter.GaussianBlur(2.2 * SCALE))
        img.alpha_composite(layer)
    draw.line(ipts, fill=(*color, 230), width=width * SCALE, joint="curve")


def iso_building(
    draw: ImageDraw.ImageDraw,
    cx: float,
    cy: float,
    w: float,
    d: float,
    h: float,
    roof: tuple[int, int, int],
    left: tuple[int, int, int],
    right: tuple[int, int, int],
    accent: tuple[int, int, int],
    tower: bool = False,
) -> None:
    top = [(cx, cy - h), (cx + w, cy - h + d * 0.5), (cx, cy - h + d), (cx - w, cy - h + d * 0.5)]
    left_face = [top[3], top[2], (cx, cy + d), (cx - w, cy + d * 0.5)]
    right_face = [top[1], (cx + w, cy + d * 0.5), (cx, cy + d), top[2]]
    shadow = [(cx - w - 10, cy + d * 0.5 + 8), (cx + w + 34, cy + d * 0.5 + 28), (cx + w + 4, cy + d + 52), (cx - w - 38, cy + d + 28)]
    draw.polygon(poly(shadow), fill=(0, 8, 17, 66))
    draw.polygon(poly(left_face), fill=left)
    draw.polygon(poly(right_face), fill=right)
    draw.polygon(poly(top), fill=roof)
    draw.line(poly(top + [top[0]]), fill=(*accent, 180), width=2 * SCALE)
    roof_inner = [
        (cx, cy - h + 10),
        (cx + w * 0.72, cy - h + d * 0.5 + 8),
        (cx, cy - h + d - 10),
        (cx - w * 0.72, cy - h + d * 0.5 - 8),
    ]
    draw.line(poly(roof_inner + [roof_inner[0]]), fill=(*accent, 74), width=1 * SCALE)
    for offset in (-0.32, 0.18):
        roof_box = [
            (cx + w * offset, cy - h + d * 0.52 - 12),
            (cx + w * (offset + 0.18), cy - h + d * 0.52 - 4),
            (cx + w * offset, cy - h + d * 0.52 + 6),
            (cx + w * (offset - 0.18), cy - h + d * 0.52 - 2),
        ]
        draw.polygon(poly(roof_box), fill=(7, 24, 38, 172), outline=(*accent, 66))

    floors = max(2, int(h // 18))
    for i in range(floors):
        y = cy - h + d * 0.58 + i * (h / (floors + 1))
        for j in range(max(2, int(w // 22))):
            x = cx - w + 18 + j * 22
            draw.line([p(x, y), p(x + 9, y + 4)], fill=(129, 222, 255, 118), width=1 * SCALE)
        for j in range(max(2, int(w // 23))):
            x = cx + 10 + j * 22
            draw.line([p(x, y + 5), p(x + 9, y + 1)], fill=(255, 213, 130, 108), width=1 * SCALE)

    if tower:
        mast_base = (cx, cy - h - 8)
        draw.line([p(*mast_base), p(cx, cy - h - 62)], fill=(*accent, 190), width=2 * SCALE)
        for r, a in [(28, 44), (48, 26), (70, 14)]:
            draw.ellipse(
                [p(cx - r, cy - h - 62 - r * 0.42), p(cx + r, cy - h - 62 + r * 0.42)],
                outline=(*accent, a),
                width=1 * SCALE,
            )
        draw.ellipse([p(cx - 6, cy - h - 68), p(cx + 6, cy - h - 56)], fill=(*accent, 220))


def campus_tree(draw: ImageDraw.ImageDraw, x: float, y: float, tone: tuple[int, int, int] = (54, 214, 107)) -> None:
    draw.line([p(x, y + 10), p(x, y - 6)], fill=(58, 48, 34, 150), width=2 * SCALE)
    draw.ellipse([p(x - 10, y - 16), p(x + 10, y + 4)], fill=(*tone, 96), outline=(*tone, 80), width=1 * SCALE)


img = Image.new("RGBA", (W * SCALE, H * SCALE), (3, 17, 28, 255))
draw = ImageDraw.Draw(img, "RGBA")

# base atmosphere
for y in range(H * SCALE):
    t = y / (H * SCALE)
    c = (
        round(2 + 5 * t),
        round(13 + 13 * t),
        round(24 + 18 * t),
        255,
    )
    draw.line([(0, y), (W * SCALE, y)], fill=c)

for cx, cy, r, col in [
    (880, 470, 620, (24, 168, 255, 38)),
    (1420, 550, 460, (54, 214, 107, 22)),
    (1600, 370, 300, (255, 77, 79, 20)),
]:
    layer = Image.new("RGBA", (W * SCALE, H * SCALE), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)
    d.ellipse([p(cx - r, cy - r * 0.58), p(cx + r, cy + r * 0.58)], fill=col)
    img.alpha_composite(layer.filter(ImageFilter.GaussianBlur(42 * SCALE)))

# distant grid
for x in range(-200, W + 260, 90):
    draw.line([p(x, 140), p(x + 520, 1120)], fill=(24, 168, 255, 18), width=1 * SCALE)
for x in range(-260, W + 200, 110):
    draw.line([p(x, 1050), p(x + 720, 120)], fill=(24, 168, 255, 15), width=1 * SCALE)

# campus boundary and roads
boundary = [(346, 274), (904, 116), (1648, 212), (1870, 484), (1610, 884), (784, 980), (270, 742)]
draw.polygon(poly(boundary), fill=(5, 30, 48, 122), outline=(24, 168, 255, 150))
glow_line(draw, boundary + [boundary[0]], (24, 168, 255), 2)

roads = [
    [(398, 645), (650, 505), (940, 480), (1250, 530), (1590, 470), (1790, 540)],
    [(650, 505), (770, 770), (1050, 840), (1380, 760), (1590, 470)],
    [(940, 480), (860, 270), (1110, 215), (1384, 302)],
    [(1030, 390), (1145, 605), (1065, 840)],
    [(500, 780), (770, 770), (1065, 840), (1425, 835)],
]
for path in roads:
    draw.line(poly(path), fill=(88, 131, 145, 80), width=18 * SCALE, joint="curve")
    draw.line(poly(path), fill=(14, 47, 68, 175), width=10 * SCALE, joint="curve")
    draw.line(poly(path), fill=(24, 168, 255, 62), width=1 * SCALE, joint="curve")

# green areas and plazas
for pts in [
    [(792, 515), (958, 430), (1120, 486), (1054, 630), (856, 632)],
    [(1320, 620), (1512, 575), (1652, 650), (1580, 780), (1372, 752)],
    [(520, 350), (700, 300), (810, 382), (662, 470)],
]:
    draw.polygon(poly(pts), fill=(16, 92, 70, 88), outline=(54, 214, 107, 72))

for x, y, r in [(990, 552, 88), (1488, 682, 96), (660, 391, 52)]:
    draw.ellipse([p(x - r, y - r * 0.48), p(x + r, y + r * 0.48)], fill=(5, 23, 34, 150), outline=(24, 168, 255, 86), width=2 * SCALE)
    draw.ellipse([p(x - r * 0.42, y - r * 0.22), p(x + r * 0.42, y + r * 0.22)], outline=(54, 214, 107, 95), width=2 * SCALE)

for tx, ty in [
    (512, 392), (535, 418), (560, 385), (620, 440), (690, 330), (730, 360),
    (1325, 650), (1365, 622), (1420, 606), (1510, 628), (1575, 720),
    (875, 610), (920, 630), (1090, 590), (1138, 575), (1460, 760),
]:
    campus_tree(draw, tx, ty)

cyan = (24, 168, 255)
green = (54, 214, 107)
amber = (255, 176, 32)
red = (255, 77, 79)

buildings = [
    (1034, 558, 92, 68, 76, (31, 70, 94), (18, 51, 72), (13, 42, 62), cyan, True),  # core
    (585, 365, 76, 56, 84, (32, 83, 105), (17, 49, 69), (14, 42, 61), green, False),  # teaching 1
    (785, 300, 68, 52, 72, (37, 86, 110), (18, 52, 72), (14, 45, 64), green, False),  # teaching 2
    (1008, 242, 86, 62, 96, (35, 80, 105), (16, 48, 69), (13, 41, 61), cyan, True),  # library
    (1390, 338, 92, 62, 88, (47, 75, 86), (28, 55, 68), (21, 48, 63), amber, False),  # lab
    (1528, 548, 70, 54, 92, (43, 69, 86), (24, 51, 68), (18, 45, 62), amber, False),  # lab b
    (1235, 720, 82, 60, 104, (30, 72, 102), (15, 45, 68), (12, 38, 58), cyan, True),  # data center
    (1588, 792, 74, 58, 108, (39, 67, 82), (21, 48, 64), (17, 42, 58), green, False),  # dorm
    (720, 784, 86, 62, 92, (37, 75, 95), (18, 49, 66), (14, 40, 58), green, False),  # admin
    (465, 700, 68, 50, 56, (42, 69, 82), (21, 47, 62), (18, 42, 55), cyan, False),  # canteen
    (1710, 447, 78, 58, 72, (61, 46, 54), (62, 31, 42), (45, 26, 40), red, True),  # soc/risk
]
for b in buildings:
    iso_building(draw, *b)

# link network
network = [
    ([(1034, 548), (1008, 242)], cyan, 3),
    ([(1034, 548), (585, 365)], green, 3),
    ([(1034, 548), (785, 300)], green, 3),
    ([(1034, 548), (1390, 338)], amber, 3),
    ([(1034, 548), (1528, 548)], amber, 3),
    ([(1034, 548), (1235, 720)], cyan, 3),
    ([(1034, 548), (720, 784)], cyan, 3),
    ([(720, 784), (465, 700)], green, 2),
    ([(1235, 720), (1588, 792)], green, 2),
    ([(1528, 548), (1710, 447)], red, 4),
    ([(1390, 338), (1710, 447)], amber, 2),
]
for pts, color, width in network:
    glow_line(draw, pts, color, width)

# probes and status points
probes = [
    (585, 310, green), (785, 260, green), (1008, 176, cyan), (1390, 284, amber),
    (1528, 494, amber), (1235, 650, cyan), (1588, 718, green), (720, 710, green),
    (465, 640, cyan), (1710, 365, red), (1034, 476, cyan), (905, 603, green),
    (1196, 500, green), (1340, 642, green), (650, 540, green), (1765, 533, red),
]
for x, y, color in probes:
    draw.ellipse([p(x - 8, y - 8), p(x + 8, y + 8)], fill=(*color, 230), outline=(226, 250, 255, 210), width=2 * SCALE)
    draw.ellipse([p(x - 22, y - 22), p(x + 22, y + 22)], outline=(*color, 54), width=2 * SCALE)

# perimeter gateway and data flow
for x, y in [(258, 748), (344, 260), (1840, 490), (1600, 900)]:
    draw.line([p(x, y), p(x, y - 78)], fill=(24, 168, 255, 160), width=2 * SCALE)
    draw.ellipse([p(x - 10, y - 86), p(x + 10, y - 66)], fill=(24, 168, 255, 200))
    for r in (28, 48, 68):
        draw.ellipse([p(x - r, y - 76 - r * 0.45), p(x + r, y - 76 + r * 0.45)], outline=(24, 168, 255, 42), width=1 * SCALE)

for i in range(14):
    sx = 80 + i * 26
    sy = 1000 + math.sin(i * 0.6) * 18
    ex = 258 + i * 5
    ey = 748 - i * 2
    draw.line([p(sx, sy), p(ex, ey)], fill=(24, 168, 255, 55), width=1 * SCALE)
for i in range(7):
    sx = 95 + i * 38
    sy = 1030 + math.cos(i * 0.8) * 16
    ex = 258 + i * 8
    ey = 770 + i * 4
    draw.line([p(sx, sy), p(ex, ey)], fill=(255, 176, 32, 58), width=1 * SCALE)

# red risk zone glow
layer = Image.new("RGBA", (W * SCALE, H * SCALE), (0, 0, 0, 0))
d = ImageDraw.Draw(layer)
d.polygon(poly([(1590, 398), (1795, 376), (1885, 492), (1782, 620), (1562, 570)]), fill=(255, 77, 79, 34), outline=(255, 77, 79, 120))
img.alpha_composite(layer.filter(ImageFilter.GaussianBlur(4 * SCALE)))
draw.polygon(poly([(1590, 398), (1795, 376), (1885, 492), (1782, 620), (1562, 570)]), outline=(255, 77, 79, 150), width=2 * SCALE)

# foreground vignette
vignette = Image.new("RGBA", (W * SCALE, H * SCALE), (0, 0, 0, 0))
vd = ImageDraw.Draw(vignette)
vd.rectangle([0, 0, W * SCALE, H * SCALE], outline=None, fill=(0, 0, 0, 0))
for r, a in [(0, 0), (1, 0)]:
    pass
edge = Image.new("L", (W * SCALE, H * SCALE), 0)
ed = ImageDraw.Draw(edge)
ed.rectangle([0, 0, W * SCALE, H * SCALE], fill=190)
ed.ellipse([p(250, 20), p(1840, 1110)], fill=0)
edge = edge.filter(ImageFilter.GaussianBlur(95 * SCALE))
vignette.putalpha(edge)
vignette_rgb = Image.new("RGBA", (W * SCALE, H * SCALE), (0, 5, 12, 190))
vignette_rgb.putalpha(edge)
img.alpha_composite(vignette_rgb)

img = img.resize((W, H), Image.Resampling.LANCZOS).convert("RGB")
OUT.parent.mkdir(parents=True, exist_ok=True)
DOC_OUT.parent.mkdir(parents=True, exist_ok=True)
img.save(OUT, quality=95)
img.save(DOC_OUT, quality=95)


def draw_world_base(size: tuple[int, int], glow: tuple[int, int, int]) -> Image.Image:
    sw, sh = size
    canvas = Image.new("RGBA", (sw * SCALE, sh * SCALE), (0, 0, 0, 0))
    d = ImageDraw.Draw(canvas, "RGBA")
    for y in range(sh * SCALE):
        t = y / (sh * SCALE)
        d.line([(0, y), (sw * SCALE, y)], fill=(3, round(18 + 8 * t), round(31 + 15 * t), 190))
    land = (16, 54, 73, 186)
    edge = (*glow, 82)
    continents = [
        [(52, 92), (88, 58), (132, 64), (168, 91), (150, 132), (95, 144), (58, 125)],
        [(185, 72), (226, 60), (268, 82), (258, 128), (212, 140), (176, 112)],
        [(288, 86), (330, 60), (388, 66), (442, 104), (420, 150), (348, 146), (302, 122)],
        [(450, 78), (526, 60), (604, 86), (650, 132), (596, 170), (500, 156), (456, 120)],
        [(520, 176), (584, 178), (624, 205), (560, 222), (506, 207)],
    ]
    for shape in continents:
        d.polygon(poly(shape), fill=land, outline=edge)
    for x in range(40, sw, 80):
        d.line([p(x, 34), p(x + 22, sh - 28)], fill=(*glow, 22), width=1 * SCALE)
    return canvas


def source_map_crop(box: tuple[float, float, float, float], size: tuple[int, int], tint: tuple[int, int, int]) -> Image.Image | None:
    if not SOURCE_THREAT_MAP.exists():
        return None
    source = Image.open(SOURCE_THREAT_MAP).convert("RGBA")
    sw, sh = source.size
    crop_box = (
        round(sw * box[0]),
        round(sh * box[1]),
        round(sw * box[2]),
        round(sh * box[3]),
    )
    crop = source.crop(crop_box).resize((size[0] * SCALE, size[1] * SCALE), Image.Resampling.LANCZOS)
    crop = ImageEnhance.Contrast(crop).enhance(1.22)
    crop = ImageEnhance.Color(crop).enhance(1.18)

    veil = Image.new("RGBA", crop.size, (2, 13, 24, 92))
    tint_layer = Image.new("RGBA", crop.size, (*tint, 22))
    crop.alpha_composite(veil)
    crop.alpha_composite(tint_layer)
    return crop


def glow_line_on(
    canvas: Image.Image,
    pts: list[tuple[float, float]],
    color: tuple[int, int, int],
    width: int = 2,
    dash: bool = False,
) -> None:
    d = ImageDraw.Draw(canvas, "RGBA")
    scaled = [p(x, y) for x, y in pts]
    for w, alpha, blur in [(12, 30, 4), (7, 62, 2), (3, 150, 1)]:
        layer = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
        ld = ImageDraw.Draw(layer, "RGBA")
        if dash:
            for start, end in zip(scaled, scaled[1:]):
                sx, sy = start
                ex, ey = end
                segments = 22
                for i in range(0, segments, 2):
                    a = i / segments
                    b = min(i + 1, segments) / segments
                    ld.line(
                        [
                            (sx + (ex - sx) * a, sy + (ey - sy) * a),
                            (sx + (ex - sx) * b, sy + (ey - sy) * b),
                        ],
                        fill=(*color, alpha),
                        width=w * SCALE,
                    )
        else:
            ld.line(scaled, fill=(*color, alpha), width=w * SCALE, joint="curve")
        canvas.alpha_composite(layer.filter(ImageFilter.GaussianBlur(blur * SCALE)))
    d.line(scaled, fill=(*color, 230), width=width * SCALE, joint="curve")


def arc_points(origin: tuple[float, float], target: tuple[float, float], lift: float) -> list[tuple[float, float]]:
    ox, oy = origin
    tx, ty = target
    mx = (ox + tx) / 2
    my = (oy + ty) / 2 - lift
    return [
        (
            (1 - t) * (1 - t) * ox + 2 * (1 - t) * t * mx + t * t * tx,
            (1 - t) * (1 - t) * oy + 2 * (1 - t) * t * my + t * t * ty,
        )
        for t in [i / 28 for i in range(29)]
    ]


def add_map_node(
    draw: ImageDraw.ImageDraw,
    x: float,
    y: float,
    color: tuple[int, int, int],
    radius: float = 5,
    rings: int = 2,
) -> None:
    for i in range(rings, 0, -1):
        r = radius + i * 10
        draw.ellipse([p(x - r, y - r), p(x + r, y + r)], outline=(*color, 42), width=1 * SCALE)
    draw.ellipse([p(x - radius, y - radius), p(x + radius, y + radius)], fill=(*color, 230), outline=(238, 250, 255, 160), width=1 * SCALE)


def draw_support_assets() -> None:
    risk_size = (720, 240)
    egress_size = (720, 240)
    probe_size = (520, 360)
    probe = Image.new("RGBA", (probe_size[0] * SCALE, probe_size[1] * SCALE), (3, 17, 28, 235))
    pd = ImageDraw.Draw(probe, "RGBA")
    for y in range(probe_size[1] * SCALE):
        t = y / (probe_size[1] * SCALE)
        pd.line([(0, y), (probe_size[0] * SCALE, y)], fill=(3, round(18 + 10 * t), round(31 + 16 * t), 235))
    boundary = [(64, 104), (160, 46), (282, 60), (446, 38), (488, 142), (416, 288), (262, 332), (92, 284), (34, 190)]
    pd.polygon(poly(boundary), fill=(8, 46, 72, 122), outline=(24, 168, 255, 120))
    glow_line_on(probe, boundary + [boundary[0]], cyan, 2)
    district_lines = [
        [(78, 110), (230, 160), (440, 74)],
        [(130, 266), (232, 160), (416, 288)],
        [(160, 46), (218, 166), (262, 332)],
        [(34, 190), (210, 210), (488, 142)],
        [(282, 60), (270, 176), (232, 300)],
    ]
    for path in district_lines:
        pd.line(poly(path), fill=(24, 168, 255, 62), width=1 * SCALE)
    for x in range(30, probe_size[0], 46):
        pd.line([p(x, 42), p(x + 28, 330)], fill=(24, 168, 255, 18), width=1 * SCALE)
    for x, y, color in [
        (96, 154, green), (150, 108, green), (224, 124, green), (356, 96, green),
        (416, 152, green), (388, 220, green), (306, 278, green), (178, 250, green),
        (112, 218, green), (374, 298, red), (260, 198, green), (216, 184, green),
        (322, 164, green), (146, 176, green), (450, 224, green), (252, 82, amber),
    ]:
        add_map_node(pd, x, y, color, 4 if color != red else 5, 1)
    probe = probe.resize(probe_size, Image.Resampling.LANCZOS).convert("RGB")

    risk = draw_world_base(risk_size, red)
    rd = ImageDraw.Draw(risk, "RGBA")
    red_wash = Image.new("RGBA", risk.size, (68, 6, 8, 56))
    risk.alpha_composite(red_wash)
    heat_points = [
        (112, 104, 34, red), (218, 92, 28, amber), (332, 102, 44, red),
        (462, 88, 30, amber), (576, 118, 42, red), (510, 174, 28, green),
        (650, 92, 36, red), (286, 158, 24, amber),
    ]
    for x, y, r, color in heat_points:
        layer = Image.new("RGBA", risk.size, (0, 0, 0, 0))
        ld = ImageDraw.Draw(layer)
        ld.ellipse([p(x - r, y - r), p(x + r, y + r)], fill=(*color, 62))
        risk.alpha_composite(layer.filter(ImageFilter.GaussianBlur(10 * SCALE)))
        add_map_node(rd, x, y, color, 4, 1)
    for x in range(42, 700, 92):
        rd.line([p(x, 40), p(x + 20, 198)], fill=(255, 77, 79, 24), width=1 * SCALE)
    risk = risk.resize(risk_size, Image.Resampling.LANCZOS).convert("RGB")

    egress = source_map_crop((0.02, 0.25, 0.92, 0.78), egress_size, cyan) or draw_world_base(egress_size, cyan)
    edraw = ImageDraw.Draw(egress, "RGBA")
    origin = (330, 128)
    flows = [
        ((88, 82), cyan, 44), ((178, 72), amber, 38), ((262, 96), cyan, 26),
        ((454, 76), cyan, 38), ((612, 112), amber, 58), ((540, 176), green, 40),
        ((208, 174), cyan, 34), ((682, 156), amber, 72),
    ]
    for target, color, lift in flows:
        glow_line_on(egress, arc_points(origin, target, lift), color, 2, dash=color == amber)
        add_map_node(edraw, target[0], target[1], color, 4, 2)
    add_map_node(edraw, origin[0], origin[1], cyan, 7, 3)
    egress = egress.resize(egress_size, Image.Resampling.LANCZOS).convert("RGB")

    for output, image in [
        (PROBE_OUT, probe),
        (DOC_PROBE_OUT, probe),
        (RISK_OUT, risk),
        (DOC_RISK_OUT, risk),
        (EGRESS_OUT, egress),
        (DOC_EGRESS_OUT, egress),
    ]:
        output.parent.mkdir(parents=True, exist_ok=True)
        image.save(output, quality=95)


draw_support_assets()
print(OUT)
print(DOC_OUT)
print(RISK_OUT)
print(EGRESS_OUT)
print(PROBE_OUT)
