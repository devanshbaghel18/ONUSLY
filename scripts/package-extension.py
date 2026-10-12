#!/usr/bin/env python3
import os
import zipfile

def package_extension():
    base_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    ext_dir = os.path.join(base_dir, 'extension')
    out_dir = os.path.join(base_dir, 'frontend', 'public', 'downloads')
    os.makedirs(out_dir, exist_ok=True)
    out_zip = os.path.join(out_dir, 'onusly-extension.zip')

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

    print(f"Packaged extension into {out_zip} ({os.path.getsize(out_zip)} bytes)")

if __name__ == '__main__':
    package_extension()
