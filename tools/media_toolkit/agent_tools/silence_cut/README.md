# Silence Remover (`silence_cut`)

Detect pauses in a recording, or cut them out automatically to make tight jump-cut edits of talks, tutorials and podcasts.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Find the silent parts of a video or audio recording (action detect, default), or remove them (action remove) to tighten talking-head videos, tutorials and podcasts with automatic jump cuts. Sound quieter than threshold_db for at least min_silence counts as silence; padding keeps a little air around speech so cuts sound natural.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the audio or video file, relative to the workspace. |
| `action` | detect \| remove |  | detect (default) lists the silences; remove cuts them out. |
| `threshold_db` | number |  | Anything quieter than this (in dB) is silence. Default -35; use -45 for quiet recordings, -25 for noisy ones. |
| `min_silence` | time |  | Shortest pause to count, default 0.6 seconds. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `padding` | time |  | Audio kept on each side of a cut, default 0.15 seconds. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `output` | string |  | Path for the result, relative to the workspace. The extension picks the format. Default: next to the input as <name>_nosilence.<ext>. An existing file is never replaced unless overwrite is true. |
| `quality` | high \| medium \| low |  | Encoding quality when re-encoding: high (visually lossless, larger files), medium, or low (smallest). Default high. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "tutorial.mp4",
  "action": "remove",
  "min_silence": "0.8"
}
```

From a terminal, in the folder that holds the files:

```bash
silence_cut --args '{"input": "tutorial.mp4", "action": "remove", "min_silence": "0.8"}'
echo '{"input": "tutorial.mp4", "action": "remove", "min_silence": "0.8"}' | silence_cut --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install silence_cut
zpilot tools install silence_cut
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
