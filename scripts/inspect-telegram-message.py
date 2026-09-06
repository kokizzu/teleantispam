#!/usr/bin/env python3
"""Fetch the Telegram web/embed variants for a message under investigation."""

from __future__ import annotations

import html
import re
import shutil
import subprocess
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


PROJECT = Path(__file__).resolve().parent.parent
REQUEST = PROJECT / "tmp" / "telegram-message-url.txt"


def variants(raw_url: str) -> list[str]:
    parsed = urllib.parse.urlparse(raw_url)
    if parsed.scheme != "https" or parsed.netloc.casefold() != "t.me":
        raise ValueError("message URL must use https://t.me/")
    parts = [part for part in parsed.path.split("/") if part]
    if len(parts) < 2:
        raise ValueError("message URL must contain a public chat and message ID")
    chat = parts[0]
    message_id = parts[-1]
    if not message_id.isdigit():
        raise ValueError("message URL must end with a numeric message ID")
    canonical = f"https://t.me/{chat}/{message_id}"
    return [
        raw_url,
        canonical,
        canonical + "?embed=1&mode=tme",
        f"https://t.me/s/{chat}?before={int(message_id) + 1}",
    ]


def extract_text(document: str) -> str:
    blocks = re.findall(
        r'<div[^>]+class="[^"]*tgme_widget_message_text[^"]*"[^>]*>(.*?)</div>',
        document,
        flags=re.IGNORECASE | re.DOTALL,
    )
    if not blocks:
        descriptions = re.findall(
            r'<meta[^>]+(?:property|name)="(?:og:description|description)"[^>]+content="([^"]*)"',
            document,
            flags=re.IGNORECASE,
        )
        blocks = descriptions
    cleaned: list[str] = []
    for block in blocks:
        text = re.sub(r"<br\s*/?>", "\n", block, flags=re.IGNORECASE)
        text = re.sub(r"<[^>]+>", "", text)
        text = html.unescape(text).strip()
        if text and text not in cleaned:
            cleaned.append(text)
    return "\n---\n".join(cleaned)


def extract_metadata(document: str) -> list[str]:
    result: list[str] = []
    for post in re.findall(r'data-post="([^"]+)"', document, flags=re.IGNORECASE):
        item = f"post={html.unescape(post)}"
        if item not in result:
            result.append(item)
    anchors = re.findall(
        r'<a[^>]+class="[^"]*(?:author|owner_name|from_author)[^"]*"[^>]+href="([^"]+)"[^>]*>(.*?)</a>',
        document,
        flags=re.IGNORECASE | re.DOTALL,
    )
    for href, label in anchors:
        label = html.unescape(re.sub(r"<[^>]+>", "", label)).strip()
        item = f"author={label or '-'} href={html.unescape(href)}"
        if item not in result:
            result.append(item)
    for user_id in re.findall(r'tg://user\?id=(\d+)', document, flags=re.IGNORECASE):
        item = f"user_id={user_id}"
        if item not in result:
            result.append(item)
    return result


def notify(message: str) -> None:
    executable = shutil.which("notify-send")
    if executable:
        subprocess.run(
            [executable, "TeleAntiSpam2Bot inspection", message],
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )


def main() -> int:
    raw_url = REQUEST.read_text(encoding="utf-8").strip()
    if not raw_url:
        raise ValueError(f"empty request: {REQUEST}")

    found = False
    for url in dict.fromkeys(variants(raw_url)):
        request = urllib.request.Request(
            url,
            headers={
                "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/140 Safari/537.36",
                "Accept-Language": "en-US,en;q=0.9,id;q=0.8",
            },
        )
        print(f"URL={url}")
        try:
            with urllib.request.urlopen(request, timeout=15) as response:
                document = response.read(4 * 1024 * 1024).decode("utf-8", errors="replace")
                print(f"STATUS={response.status} BYTES={len(document.encode('utf-8'))}")
        except (urllib.error.URLError, TimeoutError) as exc:
            print(f"ERROR={exc}")
            continue
        message = extract_text(document)
        metadata = extract_metadata(document)
        if metadata:
            print("METADATA=" + " | ".join(metadata))
        if message:
            found = True
            print("MESSAGE_BEGIN")
            print(message)
            print("MESSAGE_END")
        else:
            print("MESSAGE=not exposed by this page")

    notify("Message text captured" if found else "Telegram did not expose the message text")
    return 0 if found else 2


if __name__ == "__main__":
    raise SystemExit(main())
