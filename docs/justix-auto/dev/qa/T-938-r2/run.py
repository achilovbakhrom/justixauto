import os
import pathlib
import subprocess
import sys

root = pathlib.Path.cwd()
evidence = root / "docs/justix-auto/dev/qa/T-938-r2"
env = {key: value for key, value in os.environ.items() if not key.startswith("JUSTIXAUTO_TEST_")}
env.update(
    GOMODCACHE="/private/tmp/justixauto-t003-modcache",
    GOCACHE="/private/tmp/justixauto-integration-gocache",
    GOPROXY="off",
)
env["PATH"] = "/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin:" + env.get("PATH", "")

mode = sys.argv[1]
command = ["bash", "tools/go.sh"]
if mode == "independent":
    command += ["test", "-race", "-mod=readonly", "-count=1", "-v", "-overlay", str(evidence / "overlay.json"), "-run", "^TestQA938", "./tests/contracts"]
elif mode == "scope":
    command += ["test", "-race", "-mod=readonly", "-count=1", "-v", "./services/...", "./pkg/...", "./tests/..."]
elif mode == "vet":
    command += ["vet", "-mod=readonly", "./services/...", "./pkg/...", "./tests/..."]
else:
    raise ValueError(mode)

with (evidence / f"{mode}.log").open("wb") as output:
    result = subprocess.run(command, env=env, stdout=output, stderr=subprocess.STDOUT)
(evidence / f"{mode}-exit.txt").write_text(f"{result.returncode}\n")
raise SystemExit(result.returncode)
