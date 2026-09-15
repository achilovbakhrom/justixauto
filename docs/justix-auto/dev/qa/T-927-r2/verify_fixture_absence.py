"""Read-only independent absence checks for this QA run's exact fixture IDs."""
import json
from pathlib import Path
import re
import subprocess

log = Path("/private/tmp/justixauto-qa-t927-r2-full-race.log").read_text()
pairs = sorted(set(re.findall(r"validated task fixture removed and verified absent: ([a-f0-9]{64}) (justixauto-t927-[a-f0-9-]{36})", log)))
assert len(pairs) == 4, len(pairs)
evidence = []
for identifier, name in pairs:
    for value in (identifier, name):
        result = subprocess.run(["docker", "inspect", value], capture_output=True, text=True)
        assert result.returncode != 0 and "no such object" in result.stderr.lower(), (value, result.returncode)
    evidence.append({"id": identifier, "name": name, "id_and_name_absent": True})
print(json.dumps(evidence, indent=2))
