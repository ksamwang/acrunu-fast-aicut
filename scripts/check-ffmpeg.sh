#!/usr/bin/env sh
set -eu

ffmpeg -version | head -n 1
ffprobe -version | head -n 1
