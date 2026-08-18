#!/usr/bin/env python3
"""pqcota 데모의 topology.yaml 에서 고객 선언(declaration.json)을 만든다.

**환경을 정의하는 곳에서 선언을 끌어온다.** 노드 이름을 우리 파일에 박아 두면 상류가
토폴로지를 고칠 때 조용히 어긋난다 - 실제로 한 번 겪었다(node-web -> web-gw).

선언은 관측의 복사본이 아니다. **조직이 안다고 주장하는 것**이고, 그 주장과 관측이 어긋나는
세 모양을 보이는 것이 이 데모다. 그래서 토폴로지에서 끌어오되 **일부러 셋을 어긋내 둔다**:

  1. TLS 엣지는 선언한다              -> 관측과 만나 CONFIRMED
  2. SSH 엣지는 선언하지 않는다       -> UNDECLARED (shadow). 관리 통신은 대개 대장에 없다
  3. 선언한 엣지 하나를 뒤집어 넣는다 -> UNOBSERVED. 대장에 양방향으로 적혀 있는데
                                         실제 트래픽은 한 방향뿐인, 흔한 모양이다

셋 다 **노드 이름을 모른 채** 만들어진다. 토폴로지를 바꿔도 그대로 성립한다.

    declare.py <topology.yaml> > declaration.json
"""
import json
import re
import sys

# YAML을 통째로 읽지 않는다. 이 파일의 두 절(nodes · edges)만 본다 - 의존성을 하나 더
# 들이는 것보다, 읽는 범위를 좁히고 **어긋나면 멈추는 편**이 낫다(아래 검증).
NODE_ID = re.compile(r"^\s*-\s*id:\s*([A-Za-z0-9._-]+)")
NODE_KIND = re.compile(r"^\s*kind:\s*([A-Za-z0-9._-]+)")
EDGE = re.compile(
    r"^\s*-\s*\{\s*from:\s*([A-Za-z0-9._-]+)\s*,\s*to:\s*([A-Za-z0-9._-]+)\s*,"
    r"\s*proto:\s*([A-Za-z0-9._-]+)\s*,\s*port:\s*(\d+)"
)
SECTION = re.compile(r"^([a-z_]+):")

# 런타임별로 선언할 자산. 토폴로지의 kind 가 정한다.
ASSETS = {
    "openssl": ["libssl", "libcrypto"],
    "java": ["jca-provider-chain"],
}
# 선언할 통신. ssh 는 뺀다 - 그것이 shadow 로 드러나는 것이 이 데모의 첫 값이다.
DECLARED_PROTOS = {"pqc": "TLS", "ssl": "TLS"}


def parse(path):
    nodes, edges, section, cur = [], [], None, None
    for line in open(path, encoding="utf-8"):
        if not line.startswith((" ", "\t", "#")) and SECTION.match(line):
            section = SECTION.match(line).group(1)
        if section == "nodes":
            m = NODE_ID.match(line)
            if m:
                cur = {"id": m.group(1), "kind": None}
                nodes.append(cur)
            elif cur is not None:
                k = NODE_KIND.match(line)
                if k:
                    cur["kind"] = k.group(1)
        elif section == "edges":
            m = EDGE.match(line)
            if m:
                edges.append(
                    {"from": m.group(1), "to": m.group(2), "proto": m.group(3), "port": int(m.group(4))}
                )
    return nodes, edges


def main() -> int:
    if len(sys.argv) < 2:
        sys.exit("usage: declare.py <topology.yaml>")
    nodes, edges = parse(sys.argv[1])

    # **못 읽었으면 멈춘다.** 정규식으로 좁게 읽으므로 형식이 바뀌면 조용히 비어 나온다 -
    # 빈 선언으로 데모를 돌리면 전부 UNDECLARED가 되어 "그런 결과인가 보다" 하고 넘어간다.
    if not nodes or not edges:
        sys.exit(
            f"❌ {sys.argv[1]} 에서 노드 {len(nodes)}개 · 엣지 {len(edges)}개를 읽었다.\n"
            "   상류 토폴로지 형식이 바뀐 것 같다 - declare.py 의 정규식을 맞추라."
        )
    unknown = sorted({n["kind"] for n in nodes if n["kind"] not in ASSETS})
    if unknown:
        sys.exit(f"❌ 모르는 노드 종류: {unknown} - declare.py 의 ASSETS 에 더하라.")

    decl = {
        "_comment": (
            "pqcota 데모의 topology.yaml 에서 생성했습니다(declare.py). 직접 고치지 마십시오 - "
            "환경을 바꾸려면 그쪽 topology.yaml 을, 선언 규칙을 바꾸려면 declare.py 를 고칩니다."
        ),
        # org - 이 선언이 어느 조직의 것인가. 대조 엔진이 이 값으로 열리고, 다른 조직의
        # 자산이 섞이면 대조하지 않고 끊는다. 데모는 조직 하나뿐이라 이름만 보인다.
        "org": "demo-corp",
        "scope": [n["id"] for n in nodes],
        "nodes": [],  # extend.sh 가 실행 중 환경의 nodes.json 으로 채운다
        "assets": [
            {"node": n["id"], "runtime": "jca" if n["kind"] == "java" else n["kind"], "component": c}
            for n in nodes
            for c in ASSETS[n["kind"]]
        ],
        "edges": [],
    }

    declared = [e for e in edges if e["proto"] in DECLARED_PROTOS]
    for e in declared:
        decl["edges"].append(
            {"src": e["from"], "dst": e["to"], "port": e["port"], "proto": DECLARED_PROTOS[e["proto"]]}
        )
    # 뒤집은 하나 - 대장에는 있는데 관측되지 않는 것(UNOBSERVED).
    if declared:
        e = declared[0]
        decl["edges"].append(
            {"src": e["to"], "dst": e["from"], "port": e["port"], "proto": DECLARED_PROTOS[e["proto"]]}
        )

    json.dump(decl, sys.stdout, ensure_ascii=False, indent=2)
    print()
    print(
        f"   토폴로지에서 읽음: 노드 {len(nodes)} · 엣지 {len(edges)} "
        f"-> 선언 자산 {len(decl['assets'])} · 선언 엣지 {len(decl['edges'])}",
        file=sys.stderr,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
