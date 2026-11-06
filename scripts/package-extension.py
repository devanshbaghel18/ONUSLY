#!/usr/bin/env python3
import argparse
import hashlib
import io
import json
import os
import shutil
import struct
import subprocess
import tempfile
import zipfile

EXTENSION_ID = "jifigaejbinmdhnljobkodcgeaplojmg"

def package_extension(codebase_url=None):
    base_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    ext_dir = os.path.join(base_dir, 'extension')
    out_dir = os.path.join(base_dir, 'frontend', 'public', 'downloads')
    os.makedirs(out_dir, exist_ok=True)
    out_zip = os.path.join(out_dir, 'onusly-extension.zip')

    manifest_file = os.path.join(ext_dir, 'manifest.json')
    with open(manifest_file, 'r', encoding='utf-8') as f:
        manifest_data = json.load(f)
    ext_version = manifest_data.get("version", "1.0.0")

    if not codebase_url:
        codebase_url = os.environ.get("ONUSLY_CODEBASE_URL", "https://api.onusly.com/downloads/onusly.crx")

    include_items = [
        'manifest.json',
        'background.js',
        'blocked.html',
        'blocked.js',
        'popup.html',
        'popup.js',
        'icons',
        'src'
    ]

    with zipfile.ZipFile(out_zip, 'w', zipfile.ZIP_DEFLATED) as zipf:
        for item in include_items:
            item_path = os.path.join(ext_dir, item)
            if os.path.isdir(item_path):
                for root, _, files in os.walk(item_path):
                    for file in files:
                        full = os.path.join(root, file)
                        rel = os.path.relpath(full, ext_dir)
                        zipf.write(full, rel)
            elif os.path.isfile(item_path):
                zipf.write(item_path, item)

    print(f"Packaged extension into {out_zip} ({os.path.getsize(out_zip)} bytes, version {ext_version})")

    # Also package Firefox-compatible archive
    firefox_zip = os.path.join(out_dir, 'onusly-firefox.zip')
    with zipfile.ZipFile(firefox_zip, 'w', zipfile.ZIP_DEFLATED) as zipf:
        for item in include_items:
            item_path = os.path.join(ext_dir, item)
            if item == 'manifest.json':
                # Firefox MV3 uses background.scripts instead of background.service_worker
                ff_manifest = dict(manifest_data)
                ff_manifest["background"] = {
                    "scripts": ["background.js"],
                    "type": "module"
                }
                zipf.writestr('manifest.json', json.dumps(ff_manifest, indent=2))
            elif os.path.isdir(item_path):
                for root, _, files in os.walk(item_path):
                    for file in files:
                        full = os.path.join(root, file)
                        rel = os.path.relpath(full, ext_dir)
                        zipf.write(full, rel)
            elif os.path.isfile(item_path):
                zipf.write(item_path, item)

    print(f"Packaged Firefox extension into {firefox_zip} ({os.path.getsize(firefox_zip)} bytes)")

    # Package signed CRX for Chrome enterprise self-hosting
    key_candidates = [
        os.environ.get("ONUSLY_EXTENSION_KEY"),
        os.path.expanduser("~/.config/onusly/keys/onusly-key.pem"),
        os.path.expanduser("~/Downloads/onusly-key.pem"),
        os.path.join(base_dir, "scripts", "onusly-key.pem")
    ]
    key_path = next((k for k in key_candidates if k and os.path.exists(k)), None)

    dest_crx = os.path.join(out_dir, "onusly.crx")
    if key_path:
        # Convert to PKCS#8 format if needed
        with tempfile.NamedTemporaryFile(suffix=".pem", delete=False) as tmp_pkcs8:
            pkcs8_path = tmp_pkcs8.name

        try:
            subprocess.run([
                "openssl", "pkcs8", "-topk8", "-inform", "PEM",
                "-outform", "PEM", "-nocrypt", "-in", key_path, "-out", pkcs8_path
            ], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

            # Find Chrome/Chromium binary for official packing
            chrome_candidates = [
                os.path.expanduser("~/.local/bin/google-chrome"),
                os.path.expanduser("~/.local/bin/chromium"),
                shutil.which("google-chrome"),
                shutil.which("google-chrome-stable"),
                shutil.which("chromium"),
                shutil.which("chromium-browser")
            ]
            chrome_bin = next((b for b in chrome_candidates if b and os.path.exists(b)), None)

            if chrome_bin:
                # Stage a clean directory with only extension distribution files
                with tempfile.TemporaryDirectory() as stage_dir:
                    for item in include_items:
                        src_item = os.path.join(ext_dir, item)
                        dst_item = os.path.join(stage_dir, item)
                        if os.path.isdir(src_item):
                            shutil.copytree(src_item, dst_item)
                        elif os.path.isfile(src_item):
                            shutil.copy2(src_item, dst_item)

                    cmd = [
                        chrome_bin,
                        f"--pack-extension={stage_dir}",
                        f"--pack-extension-key={pkcs8_path}",
                        "--no-sandbox",
                        "--disable-gpu"
                    ]
                    res = subprocess.run(cmd, capture_output=True, text=True)
                    produced_crx = stage_dir + ".crx"
                    if os.path.exists(produced_crx):
                        shutil.move(produced_crx, dest_crx)
                        print(f"Packaged signed CRX into {dest_crx} ({os.path.getsize(dest_crx)} bytes)")
                    else:
                        print(f"Notice: CRX packing exited {res.returncode}: {res.stderr}")
        finally:
            if os.path.exists(pkcs8_path):
                os.remove(pkcs8_path)

    # Generate update manifest XML with synchronized version and codebase
    updates_xml_path = os.path.join(out_dir, "updates.xml")
    xml_content = f"""<?xml version='1.0' encoding='UTF-8'?>
<gupdate xmlns='http://www.google.com/update2/response' protocol='2.0'>
  <app appid='{EXTENSION_ID}'>
    <updatecheck codebase='{codebase_url}' version='{ext_version}' />
  </app>
</gupdate>
"""
    with open(updates_xml_path, 'w', encoding='utf-8') as f:
        f.write(xml_content.strip() + "\n")
    print(f"Generated enterprise update manifest into {updates_xml_path} (codebase: {codebase_url}, version: {ext_version})")

    # Artifact integrity verification
    if os.path.exists(dest_crx):
        with open(dest_crx, "rb") as f:
            data = f.read()
        magic, crx_ver, header_len = struct.unpack("<4sII", data[:12])
        assert magic == b"Cr24", "Invalid CRX magic bytes"
        assert crx_ver == 3, "Expected CRX3 format"
        zip_data = data[12 + header_len:]
        with zipfile.ZipFile(io.BytesIO(zip_data)) as z:
            crx_manifest = json.loads(z.read("manifest.json").decode())
            crx_manifest_version = crx_manifest.get("version")
            assert crx_manifest_version == ext_version, f"CRX manifest version ({crx_manifest_version}) != manifest.json version ({ext_version})"
            # Ensure no private keys or secrets packaged
            for fname in z.namelist():
                assert not fname.endswith(".pem"), f"Private key packed in CRX: {fname}"
                assert not fname.endswith(".key"), f"Key packed in CRX: {fname}"
        print(f"✅ Verified CRX3 artifact integrity: extension ID {EXTENSION_ID}, version {ext_version}, zero private keys included.")

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description="Package ONUSLY browser extension bundles.")
    parser.add_argument("--codebase-url", default=None, help="Base download URL for signed CRX in updates.xml")
    args = parser.parse_args()
    package_extension(codebase_url=args.codebase_url)
