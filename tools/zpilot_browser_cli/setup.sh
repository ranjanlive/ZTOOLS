#!/bin/sh
set -eu
package_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
prefix="${HOME}/.local/share/zpilot-browser-cli"
update_path=yes
while [ "$#" -gt 0 ]; do
    case "$1" in
        --prefix) [ "$#" -ge 2 ] || { echo '--prefix needs a path' >&2; exit 2; }; prefix=$2; shift 2 ;;
        --no-path) update_path=no; shift ;;
        --help|-h) echo 'Usage: sh setup.sh [--prefix DIR] [--no-path]'; exit 0 ;;
        *) echo "Unknown option: $1" >&2; exit 2 ;;
    esac
done
case "$(uname -s)" in Linux) target_os=linux ;; Darwin) target_os=darwin ;; *) echo 'Requires Linux or macOS' >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|amd64) target_arch=amd64 ;; aarch64|arm64) target_arch=arm64 ;; *) echo 'Unsupported architecture; available: amd64, arm64' >&2; exit 1 ;; esac
platform="$target_os-$target_arch"
case "$prefix" in /*) ;; *) prefix="$(pwd)/$prefix" ;; esac
if [ "$prefix" = "$package_root" ]; then echo 'Choose an installation directory separate from the release package' >&2; exit 1; fi
for tool in generate_image generate_music web_search webpage_read zagent_in_chrome; do
    [ -f "$package_root/$tool/binaries/$platform/$tool" ] || { echo "Missing binary: $tool ($platform)" >&2; exit 1; }
done
[ -f "$package_root/extension/manifest.json" ] || { echo 'Packaged extension is missing' >&2; exit 1; }
mkdir -p "$prefix/bin" "$prefix/tools" "$prefix/extension"
for tool in generate_image generate_music web_search webpage_read zagent_in_chrome; do
    mkdir -p "$prefix/tools/$tool"
    cp "$package_root/$tool/binaries/$platform/$tool" "$prefix/tools/$tool/$tool"
    cp "$package_root/$tool/binaries/$platform/$tool.json" "$prefix/tools/$tool/$tool.json"
    chmod +x "$prefix/tools/$tool/$tool"
    if [ ! -e "$prefix/tools/$tool/config.json" ]; then cp "$package_root/$tool/config.json" "$prefix/tools/$tool/config.json"; fi
    ln -sf "../tools/$tool/$tool" "$prefix/bin/$tool"
done
cp -R "$package_root/extension/." "$prefix/extension/"
if [ "$update_path" = yes ]; then
    # Quote the absolute path for shell startup files, including embedded apostrophes.
    quoted_bin=$(printf '%s' "$prefix/bin" | sed "s/'/'\\\\''/g")
    path_line="export PATH='$quoted_bin':\$PATH"
    for profile_file in "$HOME/.profile" "$HOME/.bashrc" "$HOME/.zshrc"; do
        if ! grep -Fqx "$path_line" "$profile_file" 2>/dev/null; then
            printf '\n# ZPilot Browser CLI\n%s\n' "$path_line" >> "$profile_file"
        fi
    done
fi
printf 'Installed %s tools in %s\n' "$platform" "$prefix"
printf 'Configs: %s/tools/<tool>/config.json (existing configs preserved)\n' "$prefix"
printf 'Load unpacked extension from: %s/extension\n' "$prefix"
printf 'Open chrome://extensions, enable Developer mode, select Load unpacked, and choose that folder. Reload if already installed.\n'
printf 'Open the extension popup and copy its Connection ID into extension_id in each tool config.\n'
printf 'Open a new terminal, then run: web_search --query "Go tutorials"\n'
printf 'No native host, browser registration, or separate service is needed.\n'
printf 'For this terminal, add %s/bin to PATH.\n' "$prefix"
