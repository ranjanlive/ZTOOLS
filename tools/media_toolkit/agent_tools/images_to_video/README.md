# Slideshow Maker (`images_to_video`)

Create a slideshow video from images with smooth transitions, any resolution (incl. vertical) and background music.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Make a video from still images: a photo slideshow, a product showcase or a video from generated images. Give a list of images or a folder (sorted naturally by name). Each image shows for seconds_per_image with a transition between them (fade, dissolve, wipes, slides, circle, zoom). Images are fitted to width×height (default 1920×1080; use 1080×1920 for vertical) with a blurred background, padding or cropping. Add background music with audio; it loops if short and fades out at the end.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `images` | string[] |  | Image files in order, relative to the workspace. |
| `folder` | string |  | Folder of images to use (jpg, png, webp, bmp, tif), sorted by name. |
| `seconds_per_image` | number |  | How long each image is shown. Default 3. |
| `transition` | fade \| none \| fadeblack \| fadewhite \| dissolve \| wipeleft \| wiperight \| slideleft \| slideright \| circleopen \| circleclose \| smoothleft \| smoothright \| zoomin |  | Transition between images. Default fade. |
| `transition_seconds` | number |  | Transition length; must be shorter than seconds_per_image. Default 0.6. |
| `width` | integer |  | Video width. Default 1920. |
| `height` | integer |  | Video height. Default 1080. |
| `fit` | blur \| pad \| crop |  | How images fill the frame: blur (default), pad or crop. |
| `fps` | integer |  | Frame rate. Default 30. |
| `audio` | string |  | Background music or narration file, relative to the workspace. |
| `output` | string |  | Path for the video, relative to the workspace. Default slideshow.mp4 next to the images. |
| `quality` | high \| medium \| low |  | Encoding quality when re-encoding: high (visually lossless, larger files), medium, or low (smallest). Default high. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "folder": "photos/trip",
  "seconds_per_image": 4,
  "transition": "fade",
  "audio": "music/calm.mp3"
}
```

From a terminal, in the folder that holds the files:

```bash
images_to_video --args '{"folder": "photos/trip", "seconds_per_image": 4, "transition": "fade", "audio": "music/calm.mp3"}'
echo '{"folder": "photos/trip", "seconds_per_image": 4, "transition": "fade", "audio": "music/calm.mp3"}' | images_to_video --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install images_to_video
zpilot tools install images_to_video
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
