# Music & Voice-over Mixer (`audio_mix`)

Add background music or a voice-over to a video with auto-ducking, looping and fade-out, or replace its soundtrack.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Combine a video (or audio) with another audio file. mode mix (default) lays the audio under the original sound, at 30% volume by default, which is right for background music; duck lowers the music automatically whenever the original audio (speech) is loud. mode replace swaps the soundtrack entirely. The added audio loops to fill the video, can start later (start_at) and fades out at the end.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | The main video or audio file, relative to the workspace. Its length decides the output length. |
| `audio` | string | yes | The music, voice-over or soundtrack to add, relative to the workspace. |
| `mode` | mix \| replace |  | mix (default) or replace. |
| `audio_volume` | number |  | Volume multiplier for the added audio. Default 0.3 for mix, 1 for replace. |
| `input_volume` | number |  | Volume multiplier for the original audio. Default 1. |
| `start_at` | time |  | When the added audio begins in the main file. Default 0. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `loop` | boolean |  | Loop the added audio if it is shorter. Default true. |
| `duck` | boolean |  | Lower the added audio while the original is loud (speech). Default false. |
| `fade_out` | time |  | Fade the added audio out over this long at the end. Default 2 s when it is cut off. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `output` | string |  | Path for the result, relative to the workspace. The extension picks the format. Default: next to the input as <name>_mixed.<ext>. An existing file is never replaced unless overwrite is true. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "vlog.mp4",
  "audio": "music/upbeat.mp3",
  "audio_volume": 0.25,
  "duck": true
}
```

From a terminal, in the folder that holds the files:

```bash
audio_mix --args '{"input": "vlog.mp4", "audio": "music/upbeat.mp3", "audio_volume": 0.25, "duck": true}'
echo '{"input": "vlog.mp4", "audio": "music/upbeat.mp3", "audio_volume": 0.25, "duck": true}' | audio_mix --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install audio_mix
zpilot tools install audio_mix
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
