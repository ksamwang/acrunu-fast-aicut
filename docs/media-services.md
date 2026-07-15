# CosyVoice3 And Remotion Deployment

## Scope

The Docker stack contains two internal media services:

- `tts`
  - Runs `Fun-CosyVoice3-0.5B-2512` on the server GPU.
  - Persists model files in the `tts-models` Docker volume.
  - Exposes `GET /healthz` and `POST /v1/synthesize` only on the Docker network and the server loopback address.
  - Requires a reference audio file for every synthesis request. `zero_shot` also requires its matching reference transcript; `instruct` requires an instruction.
- `renderer`
  - Runs Remotion with Chromium and FFmpeg on CPU.
  - Exposes `GET /healthz` and a minimal `POST /v1/render` smoke endpoint.
  - Writes its test outputs to `storage/renders/remotion` through the existing shared storage mount.

`tts` is wired to the server-side voiceover pipeline. The browser and `local-agent` never call CosyVoice directly. The API stores reference audio, queues an asynchronous task, and the worker calls `http://tts:50000/v1/synthesize`, stores the resulting WAV, then asks FunASR for normalized `narration_segments`.

`renderer` is still infrastructure-only. It is not yet called by a generation worker, and no `edit_plan` or finished video is generated in the current voiceover phase.

## Deployment

The deployment script archives committed `HEAD`, so commit the changes first. The voiceover pipeline adds database tables and task types, so deploy it with migrations:

```powershell
.\scripts\deploy-server.ps1 -RunMigrations
```

Use `-Services tts,renderer` only when the media service images themselves also need rebuilding.

The first TTS startup downloads the CosyVoice3 model into the persistent `tts-models` volume and then loads it into the GPU. It can take several minutes. The model cache remains intact across container rebuilds and server restarts.

The server `.env` can override these defaults:

```env
MODEL_DOWNLOAD_PROXY=http://10.168.10.133:10808
COSYVOICE_MODEL_ID=FunAudioLLM/Fun-CosyVoice3-0.5B-2512
COSYVOICE_FP16=true
TTS_BIND_ADDR=127.0.0.1
TTS_PORT=50000
RENDER_BIND_ADDR=127.0.0.1
RENDER_PORT=3002
RENDER_CONCURRENCY=1
```

`TTS_BIND_ADDR` and `RENDER_BIND_ADDR` must remain `127.0.0.1` unless there is an explicit reverse-proxy and authentication design. The browser must not call either service directly.

## Verification

Run these commands on the server after `docker compose up` returns:

```sh
cd /home/acrunu/acrunu-fast-aicut
docker compose ps tts renderer
curl --fail http://127.0.0.1:50000/healthz
curl --fail http://127.0.0.1:3002/healthz
nvidia-smi
```

The Remotion renderer can be exercised without any product asset or edit plan:

```sh
curl --fail --request POST http://127.0.0.1:3002/v1/render \
  --header 'Content-Type: application/json' \
  --data '{"title":"AICUT renderer smoke test","duration_in_frames":30,"fps":30}'
```

The response contains an `output_path` below `/app/storage/renders/remotion`; on the host it is available under `storage/renders/remotion`.

## Operational Notes

- The RTX 3060 has 12GB of VRAM. FunASR and CosyVoice share it. Keep `COSYVOICE_FP16=true` and do not introduce a second TTS replica on this server.
- TTS synthesis is serialized by the worker process because FunASR and CosyVoice share one GPU. LLM and VLM calls remain independently concurrent.
- The TTS container uses the standard PyTorch inference path. TensorRT and DeepSpeed are intentionally excluded because they are not required for the initial service and increase image size substantially.
- A failed or interrupted model download can be retried by restarting only `tts`; the ModelScope cache is persistent. Delete `tts-models` only when a clean model download is specifically required.
- `renderer` is deliberately renderer-only. It does not accept source filesystem paths, asset IDs, or arbitrary JavaScript. The later Go render worker must validate a persisted `edit_plan` and call a dedicated production render contract.
