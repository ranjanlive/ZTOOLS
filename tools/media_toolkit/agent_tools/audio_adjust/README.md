# Audio Enhancer (`audio_adjust`)

Clean up audio in videos or audio files: two-pass loudness normalisation (LUFS), volume, fades, noise reduction and rumble filter.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Improve the sound of a video or audio file. normalize brings it to a standard loudness (target_lufs: -14 for YouTube/Spotify, -16 for podcasts (default), -23 for broadcast) with accurate two-pass measurement. Also: volume (+6dB, -3dB, 150%), fade_in/fade_out, denoise (light/medium/strong), highpass (remove rumble below N Hz, e.g. 80) and mono. Video is copied untouched.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the audio or video file, relative to the workspace. |
| `volume` | string |  | Volume change: +6dB, -3dB, 150% or a multiplier like 1.5. |
| `normalize` | boolean |  | Normalise loudness to target_lufs with two-pass loudnorm. Default false. |
| `target_lufs` | number |  | Loudness target in LUFS, -40 to -5. Default -16. |
| `fade_in` | time |  | Fade in over this long at the start. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `fade_out` | time |  | Fade out over this long at the end. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `denoise` | none \| light \| medium \| strong |  | Background noise reduction: none (default), light, medium or strong. |
| `highpass` | integer |  | Cut rumble and hum below this frequency in Hz (80 is good for voice). |
| `mono` | boolean |  | Mix down to mono. Default false. |
| `output` | string |  | Path for the result, relative to the workspace. The extension picks the format. Default: next to the input as <name>_adjusted.<ext>. An existing file is never replaced unless overwrite is true. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "podcast_raw.wav",
  "normalize": true,
  "target_lufs": -16,
  "denoise": "light",
  "highpass": 80
}
```

From a terminal, in the folder that holds the files:

```bash
audio_adjust --args '{"input": "podcast_raw.wav", "normalize": true, "target_lufs": -16, "denoise": "light", "highpass": 80}'
echo '{"input": "podcast_raw.wav", "normalize": true, "target_lufs": -16, "denoise": "light", "highpass": 80}' | audio_adjust --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install audio_adjust
zpilot tools install audio_adjust
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
