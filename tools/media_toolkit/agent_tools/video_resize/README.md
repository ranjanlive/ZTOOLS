# Video Resizer & Reframer (`video_resize`)

Resize videos or change their aspect ratio (16:9 → 9:16 for Shorts/Reels/TikTok) by cropping, padding or a blurred background.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Resize a video or change its shape. Give width and/or height to scale, or an aspect ratio such as 9:16 (Shorts, Reels, TikTok), 1:1, 4:5 or 16:9. fit decides how the picture fills the new shape: pad (letterbox, default), crop (fill and cut the edges; focus picks which part to keep), blur (fill the bars with a blurred copy, popular for vertical video) or stretch.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video file, relative to the workspace. |
| `width` | integer |  | Target width in pixels. With only width or height, the other follows the aspect ratio. |
| `height` | integer |  | Target height in pixels. |
| `aspect` | string |  | Target aspect ratio such as 9:16, 16:9, 1:1, 4:5 or 21:9. Without width/height the size is derived from the source. |
| `fit` | pad \| crop \| blur \| stretch |  | pad (default), crop, blur or stretch. |
| `focus` | center \| left \| right \| top \| bottom |  | With fit crop: which part to keep. Default center. |
| `pad_color` | string |  | Bar color for fit pad: a name (black, white) or hex (0x1E1E1E). Default black. |
| `output` | string |  | Path for the result, relative to the workspace. The extension picks the format. Default: next to the input as <name>_<width>x<height>.<ext>. An existing file is never replaced unless overwrite is true. |
| `quality` | high \| medium \| low |  | Encoding quality when re-encoding: high (visually lossless, larger files), medium, or low (smallest). Default high. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "landscape.mp4",
  "aspect": "9:16",
  "fit": "blur",
  "height": 1920
}
```

From a terminal, in the folder that holds the files:

```bash
video_resize --args '{"input": "landscape.mp4", "aspect": "9:16", "fit": "blur", "height": 1920}'
echo '{"input": "landscape.mp4", "aspect": "9:16", "fit": "blur", "height": 1920}' | video_resize --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install video_resize
zpilot tools install video_resize
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
