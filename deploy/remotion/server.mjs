import {randomUUID} from "node:crypto";
import {mkdir} from "node:fs/promises";
import {createServer} from "node:http";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {bundle} from "@remotion/bundler";
import {renderMedia, selectComposition} from "@remotion/renderer";

const currentDirectory = path.dirname(fileURLToPath(import.meta.url));
const port = Number.parseInt(process.env.RENDER_PORT ?? "3001", 10);
const outputRoot = process.env.RENDER_OUTPUT_ROOT ?? "/app/storage/renders/remotion";
const browserExecutable = process.env.CHROME_PATH || undefined;
const concurrency = Number.parseInt(process.env.RENDER_CONCURRENCY ?? "1", 10);
const maxRequestBytes = 64 * 1024;

function sendJson(response, statusCode, body) {
  response.writeHead(statusCode, {"Content-Type": "application/json; charset=utf-8"});
  response.end(JSON.stringify(body));
}

async function readJson(request) {
  const chunks = [];
  let length = 0;

  for await (const chunk of request) {
    length += chunk.length;
    if (length > maxRequestBytes) {
      throw new Error("request body exceeds the maximum size");
    }
    chunks.push(chunk);
  }

  if (length === 0) {
    return {};
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

function boundedInteger(value, fallback, minimum, maximum) {
  if (value === undefined) {
    return fallback;
  }
  if (!Number.isInteger(value) || value < minimum || value > maximum) {
    throw new Error(`value must be an integer between ${minimum} and ${maximum}`);
  }
  return value;
}

function renderInputFrom(body) {
  const title = typeof body.title === "string" ? body.title.trim() : "AICUT renderer ready";
  if (!title || title.length > 240) {
    throw new Error("title must contain 1 to 240 characters");
  }

  return {
    title,
    durationInFrames: boundedInteger(body.duration_in_frames, 90, 1, 3_600),
    fps: boundedInteger(body.fps, 30, 1, 60),
    width: boundedInteger(body.width, 1280, 64, 3840),
    height: boundedInteger(body.height, 720, 64, 2160),
  };
}

await mkdir(outputRoot, {recursive: true});
const serveUrl = await bundle({entryPoint: path.join(currentDirectory, "src", "index.ts")});

const server = createServer(async (request, response) => {
  const requestUrl = new URL(request.url ?? "/", `http://${request.headers.host ?? "localhost"}`);

  if (request.method === "GET" && requestUrl.pathname === "/healthz") {
    sendJson(response, 200, {status: "ok", renderer: "remotion", version: "4.0.489"});
    return;
  }

  if (request.method !== "POST" || requestUrl.pathname !== "/v1/render") {
    sendJson(response, 404, {error: "not_found"});
    return;
  }

  try {
    const inputProps = renderInputFrom(await readJson(request));
    const composition = await selectComposition({
      serveUrl,
      id: "RenderSmoke",
      inputProps,
    });
    const renderId = randomUUID();
    const outputPath = path.join(outputRoot, `${renderId}.mp4`);
    await renderMedia({
      composition,
      serveUrl,
      codec: "h264",
      outputLocation: outputPath,
      inputProps,
      concurrency: Math.max(1, concurrency),
      browserExecutable,
      chromiumOptions: {
        gl: "swiftshader",
      },
    });
    sendJson(response, 201, {
      render_id: renderId,
      output_path: outputPath,
      duration_in_frames: inputProps.durationInFrames,
      fps: inputProps.fps,
    });
  } catch (error) {
    console.error("Remotion render failed", error);
    sendJson(response, 400, {error: "render_failed", message: error instanceof Error ? error.message : "render failed"});
  }
});

server.listen(port, "0.0.0.0", () => {
  console.log(`AICUT Remotion renderer listening on ${port}`);
});
