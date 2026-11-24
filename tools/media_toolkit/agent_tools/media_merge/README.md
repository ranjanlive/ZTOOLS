# Media Merger (`media_merge`)

Join videos or audio files end to end. Identical formats are joined instantly; mixed sizes, frame rates and codecs are normalised automatically.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Join several videos (or audio files) end to end into one file, in the order given. In auto mode, files with identical formats are joined instantly without re-encoding; otherwise every clip is scaled and padded to the same size and frame rate, missing audio is filled with silence, and the result is re-encoded. Use width/height/fps to choose the output format.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `inputs` | string[] | yes | Files to join, in order (2–200), relative to the workspace. |
| `output` | string |  | Path for the result, relative to the workspace. The extension picks the format; default is the first input's format. Default: next to the input as <name>_merged.<ext>. An existing file is never replaced unless overwrite is true. |
| `mode` | auto \| fast \| reencode |  | auto (default), fast (only identical formats) or reencode. |
| `width` | integer |  | Output width when re-encoding. Default: the first video's. |
| `height` | integer |  | Output height when re-encoding. Default: the first video's. |
| `fps` | number |  | Output frame rate when re-encoding. Default: the first video's. |
| `quality` | high \| medium \| low |  | Encoding quality when re-encoding: high (visually lossless, larger files), medium, or low (smallest). Default high. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "inputs": [
    "intro.mp4",
    "main.mp4",
    "outro.mp4"
  ],
  "output": "final.mp4"
}
```

From a terminal, in the folder that holds the files:

```bash
media_merge --args '{"inputs": ["intro.mp4", "main.mp4", "outro.mp4"], "output": "final.mp4"}'
echo '{"inputs": ["intro.mp4", "main.mp4", "outro.mp4"], "output": "final.mp4"}' | media_merge --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install media_merge
zpilot tools install media_merge
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
