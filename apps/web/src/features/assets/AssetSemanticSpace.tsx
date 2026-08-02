import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Object3D } from "three";
import { Button, Empty, Segmented, Space, Spin, Tag, Tooltip, Typography, message } from "antd";
import { ChevronRight, ExternalLink, Focus, ScanSearch } from "lucide-react";
import { formatDuration } from "../../shared/lib/format";
import { shotSizeLabels, sourceTypeLabels, translateValue, usabilityStatusLabels } from "../../shared/lib/labels";
import type { AssetSemanticNeighbor, AssetSemanticSpacePoint, AssetSemanticSpaceResponse } from "../../shared/types/asset";
import { getAssetSemanticNeighbors, getAssetSemanticSpace, queryAssetSemanticSpace } from "./api";

type SemanticDimension = "2d" | "3d";
type VideoShape = "portrait" | "landscape" | "square";

type AssetSemanticSpaceProps = {
  token: string;
  path: string;
  semanticQuery: string;
  productNameByID: Map<string, string>;
  onOpenAsset: (assetID: string) => Promise<void>;
};

type PointCloudProps = {
  points: AssetSemanticSpacePoint[];
  dimension: SemanticDimension;
  selectedAssetID: string;
  queryMatches: Map<string, number>;
  neighborMatches: Map<string, number>;
  resetSignal: number;
  onSelect: (point: AssetSemanticSpacePoint) => void;
  onOpen: (point: AssetSemanticSpacePoint) => void;
};

type HoveredPoint = {
  point: AssetSemanticSpacePoint;
  x: number;
  y: number;
  matchType: "query" | "neighbor" | "";
  score?: number;
};

type SemanticViewState = {
  position: [number, number, number];
  target: [number, number, number];
};

type NeighborPoint = {
  point: AssetSemanticSpacePoint;
  score: number;
};

const semanticDimensionStorageKey = "acrunu.asset-semantic-dimension";

function withQuery(path: string, key: string, value: string) {
  const [pathname, query = ""] = path.split("?", 2);
  const params = new URLSearchParams(query);
  if (value) {
    params.set(key, value);
  } else {
    params.delete(key);
  }
  const next = params.toString();
  return next ? `${pathname}?${next}` : pathname;
}

function pointTitle(point: AssetSemanticSpacePoint) {
  return point.asset_name || point.file_name || point.asset_id;
}

function pointVideoURL(point: AssetSemanticSpacePoint) {
  return `/storage/${encodeURI(point.storage_key)}`;
}

function matchMap(items: AssetSemanticNeighbor[]) {
  return new Map(items.map((item) => [item.asset_id, item.score]));
}

function scoreLabel(score?: number) {
  if (score === undefined) {
    return "";
  }
  return `${(score * 100).toFixed(1)}%`;
}

function SemanticPointCloud({ points, dimension, selectedAssetID, queryMatches, neighborMatches, resetSignal, onSelect, onOpen }: PointCloudProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const viewStateRef = useRef<Partial<Record<SemanticDimension, SemanticViewState>>>({});
  const resetVersionRef = useRef(resetSignal);
  const [hovered, setHovered] = useState<HoveredPoint | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || points.length === 0) {
      return;
    }

    let disposed = false;
    let animationFrame = 0;
    let resizeObserver: ResizeObserver | null = null;
    const cleanupCallbacks: Array<() => void> = [];
    const shouldResetView = resetVersionRef.current !== resetSignal;
    resetVersionRef.current = resetSignal;

    void Promise.all([
      import("three"),
      import("three/examples/jsm/controls/OrbitControls.js")
    ]).then(([THREE, { OrbitControls }]) => {
      if (disposed || !containerRef.current) {
        return;
      }

      const scene = new THREE.Scene();
      scene.fog = dimension === "3d" ? new THREE.FogExp2(0x081014, 0.1) : null;

      const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, powerPreference: "high-performance" });
      renderer.setClearColor(0x081014, 0);
      renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
      renderer.outputColorSpace = THREE.SRGBColorSpace;
      renderer.domElement.className = "asset-semantic-canvas";
      renderer.domElement.setAttribute("data-testid", "asset-semantic-canvas");
      container.appendChild(renderer.domElement);

      const aspect = Math.max(container.clientWidth, 1) / Math.max(container.clientHeight, 1);
      const camera = dimension === "3d"
        ? new THREE.PerspectiveCamera(42, aspect, 0.01, 100)
        : new THREE.OrthographicCamera(-1.65 * aspect, 1.65 * aspect, 1.65, -1.65, 0.01, 100);
      const storedView = shouldResetView ? undefined : viewStateRef.current[dimension];
      camera.position.set(...(storedView?.position ?? [0, 0, dimension === "3d" ? 4.4 : 4]));

      const controls = new OrbitControls(camera, renderer.domElement);
      controls.enableDamping = !window.matchMedia("(prefers-reduced-motion: reduce)").matches;
      controls.dampingFactor = 0.08;
      controls.enableRotate = dimension === "3d";
      controls.minDistance = 1.2;
      controls.maxDistance = 10;
      controls.target.set(...(storedView?.target ?? [0, 0, 0]));
      controls.update();

      const createPointTexture = (kind: "dot" | "ring") => {
        const canvas = document.createElement("canvas");
        canvas.width = 64;
        canvas.height = 64;
        const context = canvas.getContext("2d");
        if (context) {
          context.clearRect(0, 0, 64, 64);
          if (kind === "dot") {
            const glow = context.createRadialGradient(32, 32, 2, 32, 32, 30);
            glow.addColorStop(0, "rgba(255,255,255,1)");
            glow.addColorStop(0.4, "rgba(255,255,255,.96)");
            glow.addColorStop(0.72, "rgba(255,255,255,.55)");
            glow.addColorStop(1, "rgba(255,255,255,0)");
            context.fillStyle = glow;
            context.fillRect(0, 0, 64, 64);
          } else {
            context.strokeStyle = "rgba(255,255,255,1)";
            context.lineWidth = 5;
            context.beginPath();
            context.arc(32, 32, 23, 0, Math.PI * 2);
            context.stroke();
          }
        }
        const texture = new THREE.CanvasTexture(canvas);
        texture.colorSpace = THREE.SRGBColorSpace;
        return texture;
      };

      const dotTexture = createPointTexture("dot");
      const ringTexture = createPointTexture("ring");
      const geometry = new THREE.BufferGeometry();
      const positions = new Float32Array(points.length * 3);
      const colors = new Float32Array(points.length * 3);
      const hasHighlights = queryMatches.size > 0 || neighborMatches.size > 0;
      const baseVisualColor = new THREE.Color(0x3eb7d8);
      const baseSpeechColor = new THREE.Color(0xe4a34a);
      const archivedColor = new THREE.Color(0xd26464);

      points.forEach((point, index) => {
        positions[index * 3] = (dimension === "3d" ? point.x3 : point.x2) * 1.45;
        positions[index * 3 + 1] = (dimension === "3d" ? point.y3 : point.y2) * 1.45;
        positions[index * 3 + 2] = dimension === "3d" ? point.z3 * 1.45 : 0;

        const color = point.status === "archived"
          ? archivedColor.clone()
          : point.source_type === "talking_head" ? baseSpeechColor.clone() : baseVisualColor.clone();
        if (hasHighlights && !queryMatches.has(point.asset_id) && !neighborMatches.has(point.asset_id) && point.asset_id !== selectedAssetID) {
          color.multiplyScalar(0.24);
        }
        colors[index * 3] = color.r;
        colors[index * 3 + 1] = color.g;
        colors[index * 3 + 2] = color.b;
      });
      geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
      geometry.setAttribute("color", new THREE.BufferAttribute(colors, 3));
      geometry.computeBoundingSphere();

      const material = new THREE.PointsMaterial({
        size: dimension === "3d" ? 0.065 : 8,
        sizeAttenuation: dimension === "3d",
        map: dotTexture,
        alphaTest: 0.04,
        depthWrite: false,
        vertexColors: true,
        transparent: true,
        opacity: 0.9
      });
      const pointCloud = new THREE.Points(geometry, material);
      scene.add(pointCloud);

      const addMatchLayer = (matches: Map<string, number>, baseColor: number, pointSize: number) => {
        const matched = points
          .map((point, index) => ({ point, index, score: matches.get(point.asset_id) }))
          .filter((item): item is { point: AssetSemanticSpacePoint; index: number; score: number } => item.score !== undefined);
        if (matched.length === 0) {
          return;
        }
        const layerGeometry = new THREE.BufferGeometry();
        const layerPositions = new Float32Array(matched.length * 3);
        const layerColors = new Float32Array(matched.length * 3);
        matched.forEach((item, layerIndex) => {
          layerPositions[layerIndex * 3] = positions[item.index * 3];
          layerPositions[layerIndex * 3 + 1] = positions[item.index * 3 + 1];
          layerPositions[layerIndex * 3 + 2] = positions[item.index * 3 + 2];
          const strength = Math.max(0.58, Math.min(1, item.score));
          const color = new THREE.Color(baseColor).multiplyScalar(0.74 + strength * 0.26);
          layerColors[layerIndex * 3] = color.r;
          layerColors[layerIndex * 3 + 1] = color.g;
          layerColors[layerIndex * 3 + 2] = color.b;
        });
        layerGeometry.setAttribute("position", new THREE.BufferAttribute(layerPositions, 3));
        layerGeometry.setAttribute("color", new THREE.BufferAttribute(layerColors, 3));
        const layerMaterial = new THREE.PointsMaterial({
          size: pointSize,
          sizeAttenuation: dimension === "3d",
          map: ringTexture,
          alphaTest: 0.05,
          depthWrite: false,
          vertexColors: true,
          transparent: true,
          opacity: 0.96
        });
        const layer = new THREE.Points(layerGeometry, layerMaterial);
        layer.renderOrder = 2;
        scene.add(layer);
        cleanupCallbacks.push(() => {
          layerGeometry.dispose();
          layerMaterial.dispose();
        });
      };

      addMatchLayer(neighborMatches, 0x72d6b4, dimension === "3d" ? 0.105 : 13);
      addMatchLayer(queryMatches, 0xf5c451, dimension === "3d" ? 0.125 : 15);

      const selectedIndex = points.findIndex((point) => point.asset_id === selectedAssetID);
      let selectedMarker: Object3D | null = null;
      if (selectedIndex >= 0) {
        const markerMaterial = new THREE.SpriteMaterial({
          color: 0xffffff,
          map: ringTexture,
          depthTest: false,
          depthWrite: false,
          transparent: true,
          opacity: 1
        });
        selectedMarker = new THREE.Sprite(markerMaterial);
        selectedMarker.position.set(positions[selectedIndex * 3], positions[selectedIndex * 3 + 1], positions[selectedIndex * 3 + 2]);
        selectedMarker.scale.setScalar(dimension === "3d" ? 0.18 : 0.14);
        selectedMarker.renderOrder = 4;
        scene.add(selectedMarker);
        cleanupCallbacks.push(() => markerMaterial.dispose());

        const neighborLinePositions: number[] = [];
        Array.from(neighborMatches.entries())
          .sort((left, right) => right[1] - left[1])
          .slice(0, 12)
          .forEach(([assetID]) => {
            const neighborIndex = points.findIndex((point) => point.asset_id === assetID);
            if (neighborIndex < 0) {
              return;
            }
            neighborLinePositions.push(
              positions[selectedIndex * 3], positions[selectedIndex * 3 + 1], positions[selectedIndex * 3 + 2],
              positions[neighborIndex * 3], positions[neighborIndex * 3 + 1], positions[neighborIndex * 3 + 2]
            );
          });
        if (neighborLinePositions.length > 0) {
          const lineGeometry = new THREE.BufferGeometry();
          lineGeometry.setAttribute("position", new THREE.Float32BufferAttribute(neighborLinePositions, 3));
          const lineMaterial = new THREE.LineBasicMaterial({ color: 0x72d6b4, depthWrite: false, transparent: true, opacity: 0.24 });
          const lines = new THREE.LineSegments(lineGeometry, lineMaterial);
          lines.renderOrder = 1;
          scene.add(lines);
          cleanupCallbacks.push(() => {
            lineGeometry.dispose();
            lineMaterial.dispose();
          });
        }
      }

      const grid = new THREE.GridHelper(3.5, 12, 0x34505b, 0x162b32);
      if (dimension === "3d") {
        grid.position.y = -1.58;
      } else {
        grid.rotation.x = Math.PI / 2;
        grid.position.z = -0.08;
      }
      const gridMaterials = Array.isArray(grid.material) ? grid.material : [grid.material];
      gridMaterials.forEach((gridMaterial) => {
        gridMaterial.transparent = true;
        gridMaterial.opacity = dimension === "3d" ? 0.42 : 0.2;
      });
      scene.add(grid);

      const raycaster = new THREE.Raycaster();
      raycaster.params.Points = { threshold: dimension === "3d" ? 0.085 : 0.06 };
      const pointer = new THREE.Vector2();
      let pointerDown = { x: 0, y: 0 };

      const intersectPoint = (event: PointerEvent | MouseEvent) => {
        const bounds = renderer.domElement.getBoundingClientRect();
        pointer.x = ((event.clientX - bounds.left) / bounds.width) * 2 - 1;
        pointer.y = -((event.clientY - bounds.top) / bounds.height) * 2 + 1;
        raycaster.setFromCamera(pointer, camera);
        const intersection = raycaster.intersectObject(pointCloud, false)[0];
        if (!intersection || intersection.index === undefined) {
          return null;
        }
        return { point: points[intersection.index], bounds };
      };

      const handlePointerMove = (event: PointerEvent) => {
        const hit = intersectPoint(event);
        if (!hit) {
          renderer.domElement.style.cursor = "grab";
          setHovered(null);
          return;
        }
        renderer.domElement.style.cursor = "pointer";
        const queryScore = queryMatches.get(hit.point.asset_id);
        const neighborScore = neighborMatches.get(hit.point.asset_id);
        const tooltipWidth = 292;
        const tooltipHeight = hit.point.thumbnail_url ? 104 : 76;
        setHovered({
          point: hit.point,
          x: Math.max(8, Math.min(event.clientX - hit.bounds.left + 14, hit.bounds.width - tooltipWidth - 8)),
          y: Math.max(8, Math.min(event.clientY - hit.bounds.top + 14, hit.bounds.height - tooltipHeight - 8)),
          matchType: queryScore !== undefined ? "query" : neighborScore !== undefined ? "neighbor" : "",
          score: queryScore ?? neighborScore
        });
      };
      const handlePointerDown = (event: PointerEvent) => {
        pointerDown = { x: event.clientX, y: event.clientY };
      };
      const handlePointerUp = (event: PointerEvent) => {
        if (Math.hypot(event.clientX - pointerDown.x, event.clientY - pointerDown.y) > 4) {
          return;
        }
        const hit = intersectPoint(event);
        if (hit) {
          onSelect(hit.point);
        }
      };
      const handleDoubleClick = (event: MouseEvent) => {
        const hit = intersectPoint(event);
        if (hit) {
          onOpen(hit.point);
        }
      };
      renderer.domElement.addEventListener("pointermove", handlePointerMove);
      renderer.domElement.addEventListener("pointerdown", handlePointerDown);
      renderer.domElement.addEventListener("pointerup", handlePointerUp);
      renderer.domElement.addEventListener("dblclick", handleDoubleClick);
      cleanupCallbacks.push(() => renderer.domElement.removeEventListener("pointermove", handlePointerMove));
      cleanupCallbacks.push(() => renderer.domElement.removeEventListener("pointerdown", handlePointerDown));
      cleanupCallbacks.push(() => renderer.domElement.removeEventListener("pointerup", handlePointerUp));
      cleanupCallbacks.push(() => renderer.domElement.removeEventListener("dblclick", handleDoubleClick));

      const resize = () => {
        const width = Math.max(container.clientWidth, 1);
        const height = Math.max(container.clientHeight, 1);
        renderer.setSize(width, height, false);
        if (camera instanceof THREE.PerspectiveCamera) {
          camera.aspect = width / height;
        } else {
          const nextAspect = width / height;
          camera.left = -1.65 * nextAspect;
          camera.right = 1.65 * nextAspect;
          camera.top = 1.65;
          camera.bottom = -1.65;
        }
        camera.updateProjectionMatrix();
      };
      resizeObserver = new ResizeObserver(resize);
      resizeObserver.observe(container);
      resize();

      const animate = () => {
        controls.update();
        renderer.render(scene, camera);
        animationFrame = window.requestAnimationFrame(animate);
      };
      animate();

      cleanupCallbacks.push(() => {
        viewStateRef.current[dimension] = {
          position: [camera.position.x, camera.position.y, camera.position.z],
          target: [controls.target.x, controls.target.y, controls.target.z]
        };
        controls.dispose();
        geometry.dispose();
        material.dispose();
        dotTexture.dispose();
        ringTexture.dispose();
        grid.geometry.dispose();
        gridMaterials.forEach((gridMaterial) => gridMaterial.dispose());
        if (selectedMarker) {
          scene.remove(selectedMarker);
        }
        renderer.dispose();
        renderer.domElement.remove();
      });
    });

    return () => {
      disposed = true;
      window.cancelAnimationFrame(animationFrame);
      resizeObserver?.disconnect();
      cleanupCallbacks.forEach((cleanup) => cleanup());
      setHovered(null);
    };
  }, [dimension, neighborMatches, onOpen, onSelect, points, queryMatches, resetSignal, selectedAssetID]);

  return (
    <div ref={containerRef} className="asset-semantic-cloud">
      {hovered ? (
        <div className={`asset-semantic-tooltip${hovered.point.thumbnail_url ? " has-thumbnail" : ""}`} style={{ transform: `translate(${hovered.x}px, ${hovered.y}px)` }}>
          {hovered.point.thumbnail_url ? <img src={hovered.point.thumbnail_url} alt="" /> : null}
          <div>
            <div className="asset-semantic-tooltip-title">
              <strong>{pointTitle(hovered.point)}</strong>
              {hovered.score !== undefined ? <span className={`is-${hovered.matchType}`}>{scoreLabel(hovered.score)}</span> : null}
            </div>
            <p>{hovered.point.action_description || hovered.point.scene_description || "暂无语义描述"}</p>
          </div>
        </div>
      ) : null}
    </div>
  );
}

export function AssetSemanticSpace({ token, path, semanticQuery, productNameByID, onOpenAsset }: AssetSemanticSpaceProps) {
  const [result, setResult] = useState<AssetSemanticSpaceResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [dimension, setDimension] = useState<SemanticDimension>(() => window.localStorage.getItem(semanticDimensionStorageKey) === "2d" ? "2d" : "3d");
  const [selectedPoint, setSelectedPoint] = useState<AssetSemanticSpacePoint | null>(null);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [videoShape, setVideoShape] = useState<VideoShape>("portrait");
  const [videoAspectRatio, setVideoAspectRatio] = useState("9 / 16");
  const [queryMatches, setQueryMatches] = useState<Map<string, number>>(new Map());
  const [neighborMatches, setNeighborMatches] = useState<Map<string, number>>(new Map());
  const [querying, setQuerying] = useState(false);
  const [loadingNeighbors, setLoadingNeighbors] = useState(false);
  const [opening, setOpening] = useState(false);
  const [resetSignal, setResetSignal] = useState(0);
  const neighborRequestRef = useRef(0);

  const queryPath = useMemo(() => path.replace("/semantic-space", "/semantic-space/query"), [path]);

  useEffect(() => {
    window.localStorage.setItem(semanticDimensionStorageKey, dimension);
  }, [dimension]);

  useEffect(() => {
    let active = true;
    setLoading(true);
    void getAssetSemanticSpace(path, token)
      .then((response) => {
        if (!active) {
          return;
        }
        setResult(response);
        setSelectedPoint((current) => current && response.points.some((point) => point.asset_id === current.asset_id) ? current : null);
      })
      .catch((error) => {
        if (active) {
          setResult(null);
          message.error(error instanceof Error ? error.message : "加载语义空间失败");
        }
      })
      .finally(() => {
        if (active) {
          setLoading(false);
        }
      });
    return () => {
      active = false;
    };
  }, [path, token]);

  useEffect(() => {
    if (!selectedPoint) {
      setInspectorOpen(false);
      setNeighborMatches(new Map());
    }
  }, [selectedPoint]);

  useEffect(() => {
    const query = semanticQuery.trim();
    if (!query) {
      setQuerying(false);
      setQueryMatches(new Map());
      return;
    }
    let active = true;
    setQuerying(true);
    void queryAssetSemanticSpace(queryPath, query, token)
      .then((response) => {
        if (active) {
          setQueryMatches(matchMap(response.items));
        }
      })
      .catch((error) => {
        if (active) {
          setQueryMatches(new Map());
          message.error(error instanceof Error ? error.message : "语义查询失败");
        }
      })
      .finally(() => {
        if (active) {
          setQuerying(false);
        }
      });
    return () => {
      active = false;
    };
  }, [queryPath, semanticQuery, token]);

  const selectPoint = useCallback((point: AssetSemanticSpacePoint) => {
    setSelectedPoint(point);
    setInspectorOpen(true);
    setVideoShape("portrait");
    setVideoAspectRatio("9 / 16");
    setLoadingNeighbors(true);
    setNeighborMatches(new Map());
    const requestID = neighborRequestRef.current + 1;
    neighborRequestRef.current = requestID;
    const neighborPath = withQuery(path.replace("/semantic-space", `/${point.asset_id}/neighbors`), "limit", "24");
    void getAssetSemanticNeighbors(neighborPath, token)
      .then((response) => {
        if (neighborRequestRef.current === requestID) {
          setNeighborMatches(matchMap(response.items));
        }
      })
      .catch((error) => {
        if (neighborRequestRef.current === requestID) {
          setNeighborMatches(new Map());
          message.error(error instanceof Error ? error.message : "加载相邻素材失败");
        }
      })
      .finally(() => {
        if (neighborRequestRef.current === requestID) {
          setLoadingNeighbors(false);
        }
      });
  }, [path, token]);

  const openPoint = useCallback(async (point: AssetSemanticSpacePoint) => {
    setOpening(true);
    try {
      await onOpenAsset(point.asset_id);
    } finally {
      setOpening(false);
    }
  }, [onOpenAsset]);

  const points = result?.points ?? [];
  const pointByID = useMemo(() => new Map(points.map((point) => [point.asset_id, point])), [points]);
  const neighborPoints = useMemo<NeighborPoint[]>(() => Array.from(neighborMatches.entries())
    .sort((left, right) => right[1] - left[1])
    .map(([assetID, score]) => ({ point: pointByID.get(assetID), score }))
    .filter((item): item is NeighborPoint => item.point !== undefined)
    .slice(0, 6), [neighborMatches, pointByID]);
  const selectedMatchScore = selectedPoint ? queryMatches.get(selectedPoint.asset_id) : undefined;
  const visibleQueryMatches = useMemo(() => Array.from(queryMatches.keys()).filter((assetID) => pointByID.has(assetID)).length, [pointByID, queryMatches]);

  return (
    <section className={`asset-grid-card asset-semantic-space-card${inspectorOpen && selectedPoint ? " is-inspector-open" : ""}`} aria-label="素材语义空间">
      <div className="asset-semantic-workspace">
        <div className="asset-semantic-stage">
          <div className="asset-semantic-stage-toolbar">
            <div className={`asset-semantic-context${semanticQuery ? " is-query" : ""}`}>
              <ScanSearch size={17} />
              <div>
                <span>{semanticQuery ? "当前语义查询" : "当前筛选空间"}</span>
                <strong>{semanticQuery ? semanticQuery : "全部向量素材"}</strong>
              </div>
              <b>{querying ? <Spin size="small" /> : semanticQuery ? visibleQueryMatches : result?.returned ?? 0}</b>
            </div>
            <div className="asset-semantic-view-controls">
              <Segmented<SemanticDimension>
                className="asset-semantic-dimension-switch"
                size="small"
                value={dimension}
                options={[{ value: "2d", label: "2D" }, { value: "3d", label: "3D" }]}
                onChange={setDimension}
              />
              <Tooltip title="重置视角">
                <Button type="text" icon={<Focus size={17} />} aria-label="重置语义空间视角" onClick={() => setResetSignal((value) => value + 1)} />
              </Tooltip>
            </div>
          </div>

          {loading ? (
            <div className="asset-semantic-loading"><Spin size="large" /></div>
          ) : points.length === 0 ? (
            <Empty description="当前筛选下没有可展示的向量素材" />
          ) : (
            <SemanticPointCloud
              points={points}
              dimension={dimension}
              selectedAssetID={selectedPoint?.asset_id ?? ""}
              queryMatches={queryMatches}
              neighborMatches={neighborMatches}
              resetSignal={resetSignal}
              onSelect={selectPoint}
              onOpen={openPoint}
            />
          )}

          <div className="asset-semantic-stage-footer">
            <div className="asset-semantic-legend" aria-label="语义空间图例">
              <span><i className="is-visual" />纯画面</span>
              <span><i className="is-speech" />口播</span>
              <span><i className="is-query" />查询</span>
              <span><i className="is-neighbor" />近邻</span>
              <span><i className="is-archived" />归档</span>
            </div>
            <div className="asset-semantic-stats">
              {result?.sampled ? <span>抽样显示</span> : null}
              {result && result.missing_embedding_count > 0 ? <span>{result.missing_embedding_count} 项待向量化</span> : null}
              <strong>{result ? `${result.returned} / ${result.total}` : "-"}</strong>
            </div>
          </div>
        </div>

        {inspectorOpen && selectedPoint ? (
          <aside className="asset-semantic-inspector">
            <header className="asset-semantic-inspector-header">
              <div>
                <span>{productNameByID.get(selectedPoint.product_id) ?? "素材详情"}</span>
                <strong>{pointTitle(selectedPoint)}</strong>
              </div>
              <Tooltip title="收起素材检查栏">
                <Button type="text" icon={<ChevronRight size={18} />} aria-label="收起素材检查栏" onClick={() => setInspectorOpen(false)} />
              </Tooltip>
            </header>

            <div className="asset-semantic-preview">
              <div className={`asset-semantic-preview-frame is-${videoShape}`} style={{ aspectRatio: videoAspectRatio }}>
                <video
                  key={selectedPoint.asset_id}
                  src={pointVideoURL(selectedPoint)}
                  poster={selectedPoint.thumbnail_url}
                  controls
                  muted
                  preload="metadata"
                  onLoadedMetadata={(event) => {
                    const video = event.currentTarget;
                    if (!video.videoWidth || !video.videoHeight) {
                      return;
                    }
                    const ratio = video.videoWidth / video.videoHeight;
                    setVideoAspectRatio(`${video.videoWidth} / ${video.videoHeight}`);
                    setVideoShape(ratio > 1.12 ? "landscape" : ratio < 0.88 ? "portrait" : "square");
                  }}
                />
              </div>
            </div>

            <div className="asset-semantic-inspector-body">
              <div className="asset-semantic-inspector-meta">
                <Space wrap size={[5, 5]}>
                  <Tag>{translateValue(selectedPoint.source_type, sourceTypeLabels)}</Tag>
                  <Tag>{formatDuration(selectedPoint.duration_ms)}</Tag>
                  {selectedPoint.shot_size ? <Tag>{translateValue(selectedPoint.shot_size, shotSizeLabels)}</Tag> : null}
                  {selectedPoint.usability_status ? <Tag>{translateValue(selectedPoint.usability_status, usabilityStatusLabels)}</Tag> : null}
                </Space>
                {selectedMatchScore !== undefined ? <span className="asset-semantic-selected-score">查询相似度 {scoreLabel(selectedMatchScore)}</span> : null}
              </div>

              <div className="asset-semantic-copy">
                <div>
                  <Typography.Text type="secondary">动作描述</Typography.Text>
                  <p>{selectedPoint.action_description || "暂无动作描述"}</p>
                </div>
                <div>
                  <Typography.Text type="secondary">画面描述</Typography.Text>
                  <p>{selectedPoint.scene_description || "暂无画面描述"}</p>
                </div>
              </div>

              <section className="asset-semantic-neighbors">
                <div className="asset-semantic-section-heading">
                  <strong>最近邻素材</strong>
                  <span>{loadingNeighbors ? "计算中" : `${neighborMatches.size} 项`}</span>
                </div>
                {loadingNeighbors ? (
                  <div className="asset-semantic-neighbor-loading"><Spin size="small" /></div>
                ) : neighborPoints.length > 0 ? (
                  <div className="asset-semantic-neighbor-grid">
                    {neighborPoints.map(({ point, score }) => (
                      <Tooltip key={point.asset_id} title={point.action_description || point.scene_description || pointTitle(point)}>
                        <button type="button" onClick={() => selectPoint(point)} aria-label={`选择近邻素材：${pointTitle(point)}`}>
                          {point.thumbnail_url ? <img src={point.thumbnail_url} alt="" loading="lazy" /> : <span className="asset-semantic-neighbor-placeholder">无预览</span>}
                          <b>{scoreLabel(score)}</b>
                        </button>
                      </Tooltip>
                    ))}
                  </div>
                ) : (
                  <Typography.Text type="secondary">当前空间中没有可展示的近邻素材</Typography.Text>
                )}
              </section>

              <Button className="asset-semantic-open-button" type="primary" block icon={<ExternalLink size={15} />} loading={opening} onClick={() => void openPoint(selectedPoint)}>
                打开素材详情
              </Button>
            </div>
          </aside>
        ) : null}
      </div>
    </section>
  );
}
