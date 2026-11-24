# Watermark & Captions (`video_watermark`)

Overlay a logo and/or text on a video with position, size, opacity and an optional time range.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Put a logo image and/or a line of text on a video: branding, a title card, a caption or a call to action. Choose the position (corners, edges or center), margin, opacity and size, and optionally show it only between start and end. Text gets a soft background box for readability unless background is false.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video file, relative to the workspace. |
| `image` | string |  | Logo or image to overlay (PNG with transparency works best), relative to the workspace. |
| `text` | string |  | Text to draw (multiple lines allowed). |
| `position` | top_left \| top \| top_right \| left \| center \| right \| bottom_left \| bottom \| bottom_right |  | Where to place it. Default bottom_right. |
| `margin` | integer |  | Distance from the edges in pixels. Default 24. |
| `opacity` | number |  | 0.05 (faint) to 1 (solid). Default 0.85. |
| `size` | number |  | Image width as a fraction of the video width (default 0.15), or text height as a fraction of the video height (default 0.05). |
| `text_color` | string |  | Text color: a name (white, yellow) or hex (#FFCC00). Default white. |
| `background` | boolean |  | Draw a translucent box behind the text. Default true. |
| `start` | time |  | Show it from this time. Default the beginning. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `end` | time |  | Hide it after this time. Default the end. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `output` | string |  | Path for the result, relative to the workspace. The extension picks the format. Default: next to the input as <name>_watermarked.<ext>. An existing file is never replaced unless overwrite is true. |
| `quality` | high \| medium \| low |  | Encoding quality when re-encoding: high (visually lossless, larger files), medium, or low (smallest). Default high. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "promo.mp4",
  "image": "brand/logo.png",
  "position": "top_right",
  "opacity": 0.8
}
```

From a terminal, in the folder that holds the files:

```bash
video_watermark --args '{"input": "promo.mp4", "image": "brand/logo.png", "position": "top_right", "opacity": 0.8}'
echo '{"input": "promo.mp4", "image": "brand/logo.png", "position": "top_right", "opacity": 0.8}' | video_watermark --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install video_watermark
zpilot tools install video_watermark
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
