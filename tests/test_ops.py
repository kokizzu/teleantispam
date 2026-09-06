#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import subprocess
import unittest
from pathlib import Path


PROJECT = Path(__file__).resolve().parent.parent


def load_script(name: str):
    path = PROJECT / "scripts" / name
    spec = importlib.util.spec_from_file_location(path.stem, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


INSPECT = load_script("inspect-telegram-message.py")
MANUAL = load_script("remote_manual_moderation.py")
RELEASE = load_script("release_changes.py")


class TelegramMessageInspectorTest(unittest.TestCase):
    def test_topic_link_generates_canonical_and_embed_variants(self):
        urls = INSPECT.variants("https://t.me/gophers_id/1/70374")
        self.assertIn("https://t.me/gophers_id/70374", urls)
        self.assertIn("https://t.me/gophers_id/70374?embed=1&mode=tme", urls)

    def test_extracts_message_and_sender_metadata(self):
        document = '''
        <div data-post="gophers_id/70374">
          <a class="tgme_widget_message_author_name" href="https://t.me/spam_sender"><span>Spam Sender</span></a>
          <div class="tgme_widget_message_text">Finance Company<br>https://t.me/+private</div>
        </div>
        '''
        self.assertEqual("Finance Company\nhttps://t.me/+private", INSPECT.extract_text(document))
        self.assertEqual(
            ["post=gophers_id/70374", "author=Spam Sender href=https://t.me/spam_sender"],
            INSPECT.extract_metadata(document),
        )


class ManualModerationWorkflowTest(unittest.TestCase):
    def test_request_hash_is_canonical(self):
        left = {"chat_id": -1, "user_id": 2, "message_ids": [3]}
        right = {"message_ids": [3], "user_id": 2, "chat_id": -1}
        self.assertEqual(MANUAL.sha256(left), MANUAL.sha256(right))

    def test_release_request_uses_direct_master_and_explicit_paths(self):
        request = RELEASE.load_request()
        self.assertEqual("master", request["branch"])
        self.assertEqual(len(request["paths"]), len(set(request["paths"])))
        self.assertTrue(all(not path.startswith("tmp/") for path in request["paths"]))

    def test_shell_wrappers_parse(self):
        for name in ("inspect-worktree.sh", "remote-find-message.sh", "remote-manual-moderate.sh", "remote-status.sh"):
            result = subprocess.run(
                ["bash", "-n", str(PROJECT / "scripts" / name)],
                check=False,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            self.assertEqual(0, result.returncode, result.stderr)

    def test_remote_status_redacts_telegram_api_tokens(self):
        source = (PROJECT / "scripts" / "remote-status.sh").read_text(encoding="utf-8")
        self.assertIn("api\\.telegram\\.org/bot", source)
        self.assertIn("<redacted>", source)


if __name__ == "__main__":
    unittest.main()
