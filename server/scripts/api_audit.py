#!/usr/bin/env python3
"""接口清单 ↔ 路由 双向对账。

用途：每阶段收尾跑一次，确保
  ① 文档里登记的接口都有对应的路由（未实现的功能不写死路径，只留在功能章节）
  ② 代码里新增的接口都登记进了清单（防止 me 接口那种漂移）

用法： python scripts/api_audit.py
退出码：0 = 双向零漂移；1 = 任一侧有漂移（含「文档有、代码无」）
"""
import io
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DOC = os.path.join(ROOT, "..", "docs", "features.md")
ROUTER = os.path.join(ROOT, "internal", "router", "router.go")

# 组名必须与 router.go 里的 gin.Group 变量名逐一对应。
# appAuth 是公开接口块里单独开的一组成员态路由（前台登出不要求已入组），
# 漏了它就会把 /api/app/logout 误报成「文档有、代码无」。
GROUPS = [
    ("api", "/api"),
    ("admin", "/api/admin"),
    ("app", "/api/app"),
    ("appAuth", "/api/app"),
    ("inFamily", "/api/app"),
    ("device", "/api/device"),
]


def doc_paths():
    text = io.open(DOC, encoding="utf-8").read()
    out = []
    # 只解析表格首列：编号允许字母后缀（如 15.1.3a），否则「logout」这类补登记会被误报成缺文档。
    # 锚定行首后，正文/说明里出现的「15.2.4 xxx」不会被误当成接口登记。
    for num, cell in re.findall(r"^\| (15\.[1-4]\.\d+[a-z]?) \|(.+?)\|", text, re.M):
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
    print(f"\n[文档有、代码无] {len(missing)} 条（必须为 0：未实现的功能不写死接口路径）")
    for n, p in missing:
        print(f"  {n:9} {p}")
    print(f"\n[代码有、文档无] {len(extra)} 条（必须为 0）")
    for m, p in extra:
        print(f"  {m:6} {p}")

    return 1 if (extra or missing) else 0


if __name__ == "__main__":
    sys.exit(main())
