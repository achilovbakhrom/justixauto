from pathlib import Path
from PIL import Image, ImageChops
import json

root = Path(__file__).parent
records = []
for width in [1440, 1180, 600]:
    reference = Image.open(root / f'independent-r2-{width}-reference-projected.png').convert('RGB')
    candidate = Image.open(root / f'independent-r2-{width}-candidate-viewport.png').convert('RGB')
    for name, box in [('sidebar', (0, 0, 244, 1000)), ('visible-header', (244, 0, width, 64))]:
        diff = ImageChops.difference(reference.crop(box), candidate.crop(box))
        count = sum(any(channel for channel in pixel) for pixel in diff.getdata())
        records.append({'width': width, 'region': name, 'box': box, 'differingPixels': count})
        assert count <= 30 if name == 'sidebar' else count == 0
print(json.dumps(records, indent=2))
