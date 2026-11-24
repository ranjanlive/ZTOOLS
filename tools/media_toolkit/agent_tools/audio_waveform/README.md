# Waveform & Audiogram (`audio_waveform`)

Draw audio as a waveform or spectrogram image, or render an audiogram video (animated waveform over an image) for social media.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Visualise audio. kind waveform (default) draws a PNG of the whole waveform, spectrogram draws frequencies over time (useful for spotting noise, hum or clipping), and audiogram renders an MP4 with an animated waveform over a background image, the usual way to share podcast clips on social media. Images are attached to the result for vision models.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the audio or video file, relative to the workspace. |
| `kind` | waveform \| spectrogram \| audiogram |  | waveform (default), spectrogram or audiogram. |
| `width` | integer |  | Width in pixels. Default 1600 (1080 for audiograms). |
| `height` | integer |  | Height in pixels. Default 400 waveform, 600 spectrogram, 1080 audiogram. |
| `color` | string |  | Waveform color: a name or hex (#4F8EF7, default). |
| `background_image` | string |  | For audiograms: background image (cover art), relative to the workspace. |
| `output` | string |  | Path for the result, relative to the workspace. PNG for images, MP4 for audiograms. Default: next to the input as <name>_<kind>.<ext>. An existing file is never replaced unless overwrite is true. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "episode_clip.mp3",
  "kind": "audiogram",
  "background_image": "cover.jpg"
}
```

From a terminal, in the folder that holds the files:

```bash
audio_waveform --args '{"input": "episode_clip.mp3", "kind": "audiogram", "background_image": "cover.jpg"}'
echo '{"input": "episode_clip.mp3", "kind": "audiogram", "background_image": "cover.jpg"}' | audio_waveform --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install audio_waveform
zpilot tools install audio_waveform
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
