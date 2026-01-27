#!/usr/bin/env bash
set -euo pipefail

json_python_cmd() {
  if command -v python >/dev/null 2>&1; then
    echo "python"
    return
  fi
  if command -v python3 >/dev/null 2>&1; then
    echo "python3"
    return
  fi
  if command -v py >/dev/null 2>&1; then
    echo "py"
    return
  fi
  return 1
}

json_pretty() {
  if command -v jq >/dev/null 2>&1; then
    jq .
    return
  fi
  local py
  if py=$(json_python_cmd); then
    if [[ "$py" == "py" ]]; then
      "$py" -3 -m json.tool
    else
      "$py" -m json.tool
    fi
    return
  fi
  cat
}

json_get() {
  local expr="$1"
  if command -v jq >/dev/null 2>&1; then
    jq -r "$expr"
    return
  fi
  local py
  if py=$(json_python_cmd); then
    if [[ "$py" == "py" ]]; then
      "$py" -3 - "$expr" <<'PY'
import json
import sys

expr = sys.argv[1]
data = json.load(sys.stdin)

def extract(node, path):
    if path.startswith("."):
        path = path[1:]
    while path:
        if path.startswith("["):
            end = path.index("]")
            idx = int(path[1:end])
            node = node[idx]
            path = path[end + 1:]
            if path.startswith("."):
                path = path[1:]
            continue
        key = []
        i = 0
        while i < len(path) and path[i] not in ".[":
            key.append(path[i])
            i += 1
        key = "".join(key)
        if key:
            node = node[key]
        path = path[i:]
        if path.startswith("."):
            path = path[1:]
    return node

result = extract(data, expr)
if isinstance(result, (dict, list)):
    print(json.dumps(result))
elif result is None:
    print("")
else:
    print(result)
PY
    else
      "$py" - "$expr" <<'PY'
import json
import sys

expr = sys.argv[1]
data = json.load(sys.stdin)

def extract(node, path):
    if path.startswith("."):
        path = path[1:]
    while path:
        if path.startswith("["):
            end = path.index("]")
            idx = int(path[1:end])
            node = node[idx]
            path = path[end + 1:]
            if path.startswith("."):
                path = path[1:]
            continue
        key = []
        i = 0
        while i < len(path) and path[i] not in ".[":
            key.append(path[i])
            i += 1
        key = "".join(key)
        if key:
            node = node[key]
        path = path[i:]
        if path.startswith("."):
            path = path[1:]
    return node

result = extract(data, expr)
if isinstance(result, (dict, list)):
    print(json.dumps(result))
elif result is None:
    print("")
else:
    print(result)
PY
    fi
    return
  fi
  return 1
}
