#!/usr/bin/env python3
"""接口清单 ↔ 路由 双向对账。

用途：每阶段收尾跑一次，确保
  ① 文档里写的接口没有「写了却没实现」的漏项（属于后续阶段的除外）
  ② 代码里新增的接口都登记进了清单（防止 me 接口那种漂移）

用法： python scripts/api_audit.py
退出码：0 = 无「代码有、文档无」；1 = 存在未登记接口
"""
import io
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DOC = os.path.join(ROOT, "..", "docs", "features.md")
ROUTER = os.path.join(ROOT, "internal", "router", "router.go")

GROUPS = [
    ("api", "/api"),
    ("admin", "/api/admin"),
    ("app", "/api/app"),
    ("inFamily", "/api/app"),
    ("device", "/api/device"),
]


def doc_paths():
    text = io.open(DOC, encoding="utf-8").read()
    out = []
    for num, cell in re.findall(r"\| (15\.[1-4]\.\d+) \|(.+?)\|", text):
        for p in re.findall(r"`(/api[^`]*)`", cell):
            if (num, p) not in out:
                out.append((num, p))
    return out


def impl_paths():
    text = io.open(ROUTER, encoding="utf-8").read()
    out = []
    for grp, prefix in GROUPS:
        for m, p in re.findall(
            r"\b" + grp + r"\.(GET|POST|PUT|DELETE)\(\"([^\"]*)\"", text
        ):
            out.append((m, prefix + p if p else prefix))
    return out


def main():
    dp, ip = doc_paths(), impl_paths()
    doc_set, impl_set = {p for _, p in dp}, {p for _, p in ip}

    missing = [(n, p) for n, p in dp if p not in impl_set]
    extra = [(m, p) for m, p in sorted(set(ip)) if p not in doc_set]

    print(f"文档登记 {len(doc_set)} 条，代码实现 {len(impl_set)} 条")
    print(f"\n[文档有、代码无] {len(missing)} 条（应全部属于后续阶段）")
    for n, p in missing:
        print(f"  {n:9} {p}")
    print(f"\n[代码有、文档无] {len(extra)} 条（必须为 0）")
    for m, p in extra:
        print(f"  {m:6} {p}")

    return 1 if extra else 0


if __name__ == "__main__":
    sys.exit(main())
