import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Material, Mesh } from "three";
import { Button, Card, Empty, Segmented, Space, Spin, Tag, Tooltip, Typography, message } from "antd";
import { Box, ExternalLink, Focus, ScanSearch } from "lucide-react";
import { formatDuration } from "../../shared/lib/format";
import { shotSizeLabels, sourceTypeLabels, translateValue, usabilityStatusLabels } from "../../shared/lib/labels";
import type { AssetSemanticNeighbor, AssetSemanticSpacePoint, AssetSemanticSpaceResponse } from "../../shared/types/asset";
import { getAssetSemanticNeighbors, getAssetSemanticSpace, queryAssetSemanticSpace } from "./api";

type SemanticDimension = "2d" | "3d";

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
};

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

function SemanticPointCloud({ points, dimension, selectedAssetID, queryMatches, neighborMatches, resetSignal, onSelect, onOpen }: PointCloudProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
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

    void Promise.all([
      import("three"),
      import("three/examples/jsm/controls/OrbitControls.js")
    ]).then(([THREE, { OrbitControls }]) => {
      if (disposed || !containerRef.current) {
        return;
      }

      const scene = new THREE.Scene();
      scene.background = new THREE.Color(0x071117);
      scene.fog = dimension === "3d" ? new THREE.FogExp2(0x071117, 0.11) : null;

      const renderer = new THREE.WebGLRenderer({ antialias: true, powerPreference: "high-performance" });
      renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
      renderer.outputColorSpace = THREE.SRGBColorSpace;
      renderer.domElement.className = "asset-semantic-canvas";
      renderer.domElement.setAttribute("data-testid", "asset-semantic-canvas");
      container.appendChild(renderer.domElement);

      const aspect = Math.max(container.clientWidth, 1) / Math.max(container.clientHeight, 1);
      const camera = dimension === "3d"
        ? new THREE.PerspectiveCamera(42, aspect, 0.01, 100)
        : new THREE.OrthographicCamera(-1.65 * aspect, 1.65 * aspect, 1.65, -1.65, 0.01, 100);
      camera.position.set(0, 0, dimension === "3d" ? 4.4 : 4);

      const controls = new OrbitControls(camera, renderer.domElement);
      controls.enableDamping = true;
      controls.dampingFactor = 0.08;
      controls.enableRotate = dimension === "3d";
      controls.minDistance = 1.2;
      controls.maxDistance = 10;
      controls.target.set(0, 0, 0);
      controls.update();

      const geometry = new THREE.BufferGeometry();
      const positions = new Float32Array(points.length * 3);
      const colors = new Float32Array(points.length * 3);
      const hasHighlights = queryMatches.size > 0 || neighborMatches.size > 0;
      const baseVisualColor = new THREE.Color(0x38bdf8);
      const baseSpeechColor = new THREE.Color(0xf59e0b);
      const archivedColor = new THREE.Color(0xef4444);
      const queryColor = new THREE.Color(0xfacc15);
      const neighborColor = new THREE.Color(0x34d399);
      const selectedColor = new THREE.Color(0xffffff);

      points.forEach((point, index) => {
        positions[index * 3] = (dimension === "3d" ? point.x3 : point.x2) * 1.45;
        positions[index * 3 + 1] = (dimension === "3d" ? point.y3 : point.y2) * 1.45;
        positions[index * 3 + 2] = dimension === "3d" ? point.z3 * 1.45 : 0;

        let color = point.status === "archived"
          ? archivedColor.clone()
          : point.source_type === "talking_head" ? baseSpeechColor.clone() : baseVisualColor.clone();
        if (hasHighlights) {
          color.multiplyScalar(0.2);
        }
        if (neighborMatches.has(point.asset_id)) {
          color = neighborColor.clone();
        }
        if (queryMatches.has(point.asset_id)) {
          color = queryColor.clone();
        }
        if (point.asset_id === selectedAssetID) {
          color = selectedColor.clone();
        }
        colors[index * 3] = color.r;
        colors[index * 3 + 1] = color.g;
        colors[index * 3 + 2] = color.b;
      });
      geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
      geometry.setAttribute("color", new THREE.BufferAttribute(colors, 3));
      geometry.computeBoundingSphere();

      const material = new THREE.PointsMaterial({
        size: dimension === "3d" ? 0.04 : 4.5,
        sizeAttenuation: dimension === "3d",
        vertexColors: true,
        transparent: true,
        opacity: 0.94
      });
      const pointCloud = new THREE.Points(geometry, material);
      scene.add(pointCloud);

      const selectedIndex = points.findIndex((point) => point.asset_id === selectedAssetID);
      let selectedMarker: Mesh | null = null;
      if (selectedIndex >= 0) {
        const markerGeometry = dimension === "3d"
          ? new THREE.SphereGeometry(0.042, 16, 12)
          : new THREE.RingGeometry(0.03, 0.045, 24);
        const markerMaterial = new THREE.MeshBasicMaterial({ color: 0xffffff, transparent: true, opacity: 0.9, side: THREE.DoubleSide });
        selectedMarker = new THREE.Mesh(markerGeometry, markerMaterial);
        selectedMarker.position.set(positions[selectedIndex * 3], positions[selectedIndex * 3 + 1], positions[selectedIndex * 3 + 2]);
        scene.add(selectedMarker);
      }

      if (dimension === "3d") {
        const grid = new THREE.GridHelper(3.3, 10, 0x284756, 0x15303c);
        grid.position.y = -1.55;
        scene.add(grid);
      }

      const raycaster = new THREE.Raycaster();
      raycaster.params.Points = { threshold: dimension === "3d" ? 0.075 : 0.055 };
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
        setHovered({ point: hit.point, x: event.clientX - hit.bounds.left + 12, y: event.clientY - hit.bounds.top + 12 });
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
        controls.dispose();
        geometry.dispose();
        material.dispose();
        if (selectedMarker) {
          selectedMarker.geometry.dispose();
          (selectedMarker.material as Material).dispose();
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
        <div className="asset-semantic-tooltip" style={{ transform: `translate(${hovered.x}px, ${hovered.y}px)` }}>
          <strong>{pointTitle(hovered.point)}</strong>
          <span>{hovered.point.action_description || hovered.point.scene_description || "暂无语义描述"}</span>
        </div>
      ) : null}
    </div>
  );
}

export function AssetSemanticSpace({ token, path, semanticQuery, productNameByID, onOpenAsset }: AssetSemanticSpaceProps) {
  const [result, setResult] = useState<AssetSemanticSpaceResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [dimension, setDimension] = useState<SemanticDimension>("3d");
  const [selectedPoint, setSelectedPoint] = useState<AssetSemanticSpacePoint | null>(null);
  const [queryMatches, setQueryMatches] = useState<Map<string, number>>(new Map());
  const [neighborMatches, setNeighborMatches] = useState<Map<string, number>>(new Map());
  const [querying, setQuerying] = useState(false);
  const [loadingNeighbors, setLoadingNeighbors] = useState(false);
  const [opening, setOpening] = useState(false);
  const [resetSignal, setResetSignal] = useState(0);

  const queryPath = useMemo(() => path.replace("/semantic-space", "/semantic-space/query"), [path]);

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
    const query = semanticQuery.trim();
    if (!query) {
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
    setLoadingNeighbors(true);
    const neighborPath = withQuery(path.replace("/semantic-space", `/${point.asset_id}/neighbors`), "limit", "24");
    void getAssetSemanticNeighbors(neighborPath, token)
      .then((response) => setNeighborMatches(matchMap(response.items)))
      .catch((error) => {
        setNeighborMatches(new Map());
        message.error(error instanceof Error ? error.message : "加载相邻素材失败");
      })
      .finally(() => setLoadingNeighbors(false));
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
  const headerExtra = (
    <Space size={8}>
      <Typography.Text type="secondary">{result ? `${result.returned} / ${result.total} 点` : "-"}</Typography.Text>
      <Segmented<SemanticDimension>
        size="small"
        value={dimension}
        options={[{ value: "2d", label: "2D" }, { value: "3d", label: "3D" }]}
        onChange={setDimension}
      />
      <Tooltip title="重置视角">
        <Button size="small" type="text" icon={<Focus size={16} />} aria-label="重置语义空间视角" onClick={() => setResetSignal((value) => value + 1)} />
      </Tooltip>
    </Space>
  );

  return (
    <Card className="asset-grid-card asset-semantic-space-card" title={<Space size={8}><Box size={17} />语义空间</Space>} extra={headerExtra}>
      <div className="asset-semantic-meta">
        <div className="asset-semantic-legend" aria-label="语义空间图例">
          <span><i className="is-visual" />纯画面</span>
          <span><i className="is-speech" />口播</span>
          <span><i className="is-query" />查询结果</span>
          <span><i className="is-neighbor" />最近邻</span>
          <span><i className="is-archived" />已归档</span>
        </div>
        <Space size={8} wrap>
          {querying ? <Tag icon={<ScanSearch size={12} />} color="gold">检索中</Tag> : null}
          {result?.sampled ? <Tag color="blue">已抽样显示</Tag> : null}
          {result && result.missing_embedding_count > 0 ? <Tag>{result.missing_embedding_count} 项待向量化</Tag> : null}
        </Space>
      </div>
      <div className="asset-semantic-workspace">
        <div className="asset-semantic-stage">
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
        </div>
        <aside className="asset-semantic-inspector">
          {selectedPoint ? (
            <>
              <div className="asset-semantic-preview">
                <video
                  key={selectedPoint.asset_id}
                  src={pointVideoURL(selectedPoint)}
                  poster={selectedPoint.thumbnail_url}
                  controls
                  muted
                  preload="metadata"
                />
              </div>
              <div className="asset-semantic-inspector-body">
                <div className="asset-semantic-inspector-heading">
                  <div>
                    <Typography.Title level={5}>{pointTitle(selectedPoint)}</Typography.Title>
                    <Typography.Text type="secondary">{productNameByID.get(selectedPoint.product_id) ?? selectedPoint.product_id}</Typography.Text>
                  </div>
                  <Tooltip title="打开素材详情">
                    <Button loading={opening} type="text" icon={<ExternalLink size={16} />} aria-label="打开素材详情" onClick={() => void openPoint(selectedPoint)} />
                  </Tooltip>
                </div>
                <Space wrap size={[5, 5]}>
                  <Tag>{translateValue(selectedPoint.source_type, sourceTypeLabels)}</Tag>
                  <Tag>{formatDuration(selectedPoint.duration_ms)}</Tag>
                  {selectedPoint.shot_size ? <Tag>{translateValue(selectedPoint.shot_size, shotSizeLabels)}</Tag> : null}
                  {selectedPoint.usability_status ? <Tag>{translateValue(selectedPoint.usability_status, usabilityStatusLabels)}</Tag> : null}
                </Space>
                <div className="asset-semantic-copy">
                  <Typography.Text type="secondary">画面</Typography.Text>
                  <p>{selectedPoint.scene_description || "暂无画面描述"}</p>
                  <Typography.Text type="secondary">动作</Typography.Text>
                  <p>{selectedPoint.action_description || "暂无动作描述"}</p>
                </div>
                <Typography.Text className="asset-semantic-neighbor-count" type="secondary">
                  {loadingNeighbors ? "正在计算最近邻" : `已高亮 ${neighborMatches.size} 个最近邻`}
                </Typography.Text>
                <Button type="primary" block icon={<ExternalLink size={15} />} loading={opening} onClick={() => void openPoint(selectedPoint)}>
                  打开素材详情
                </Button>
              </div>
            </>
          ) : (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="未选择素材" />
          )}
        </aside>
      </div>
    </Card>
  );
}
