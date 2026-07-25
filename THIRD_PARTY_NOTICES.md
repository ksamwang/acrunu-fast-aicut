# Third-Party Notices

Copyright (C) 2026 ACRUNU Fast Cut contributors.

The original source code in this repository is licensed under the GNU General Public License version 3 only (`GPL-3.0-only`). See `LICENSE` for the complete terms.

This project uses, downloads, builds, or interoperates with third-party software and data. Those components remain under their own licenses. The project license does not replace, extend, or override any third-party license.

## Distributed and Integrated Components

### FFmpeg and FFprobe

The Windows Local Agent installer includes an external FFmpeg distribution. The installer preserves the license and README supplied with that distribution. FFmpeg licensing depends on the configuration and enabled libraries of the selected build; distributors must retain the bundled notices and comply with the terms reported by that build.

- Project: https://ffmpeg.org/
- License information: https://ffmpeg.org/legal.html

### Remotion

The renderer uses Remotion packages pinned in `deploy/remotion/package-lock.json`. These packages declare `SEE LICENSE IN LICENSE.md` and are governed by the Remotion license, not by this project's GPL declaration. Review the upstream terms before operating or redistributing the renderer.

- Project: https://www.remotion.dev/
- Source: https://github.com/remotion-dev/remotion

### FunASR and CosyVoice

The ASR image installs FunASR and related Python packages. The TTS image fetches a pinned CosyVoice source revision. Their source code, transitive dependencies, downloaded models, and model weights are governed by their respective upstream licenses and model terms.

- FunASR: https://github.com/modelscope/FunASR
- CosyVoice: https://github.com/FunAudioLLM/CosyVoice

### Fonts and Package Dependencies

The web application includes Noto Sans SC through `@fontsource/noto-sans-sc`; the font is distributed under the SIL Open Font License 1.1. JavaScript and Go dependencies retain the licenses declared by their upstream projects. Exact dependency versions are recorded in `package-lock.json` files and `go.sum`.

## Models, Services, and User Content

Model weights, hosted model APIs, reference audio, product images, video, music, subtitles, and other user-provided or generated content are not relicensed under GPL merely because they are processed by this software. Operators and distributors are responsible for obtaining the rights required for those materials and services.

## Trademarks

The GNU GPL grants rights to copyrighted software. It does not grant trademark rights in the `ACRUNU Fast Cut` name, logos, product identity, or other brand assets.
